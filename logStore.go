package raft

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/vmihailenco/msgpack"
)

var BREAK_SYMBOL = []byte("§")

const (
	LOG_DIR     string = "./node_data/logs"
	HYPEN       string = "-"
	FILE_FORMAT string = "node_data/logs/%v-%v"
)

var (
	ErrNoLogs         = errors.New("there are no logs")
	ErrLogFormatWrong = errors.New("invalid log file parsed: not matching #-# format")
	ErrKeyNotFound    = errors.New("key not found")
)

type Log struct {
	Term           uint64
	Index          uint64
	LogType        uint64
	Data           []byte
	LeaderCommited bool
}

type LogStore interface {
	AppendLog(*Log) error
	GetLog(uint64) (*Log, error)
	SetLog(uint64, *Log) error
	UpdateCommited(uint64) (bool, error)
	IncrementIndex()
	IncrementTerm()
	GetLatestIndex() uint64
	GetLatestTerm() uint64
	RestoreLogs(ApplicationApply) error
	IsPiping() bool
	SetPiping(bool)
	ApplyFrom(uint64, ApplicationApply)
}

type logStore struct {
	logs   *safeMap
	index  uint64
	term   uint64
	piping bool

	threshold  uint64
	currBatch  uint64
	persistMux *sync.Mutex
}

func NewLogStore() (LogStore, error) {
	// log directory - Create a folder/directory at a full qualified path
	err := os.MkdirAll(LOG_DIR, 0755)
	if err != nil && !strings.Contains(err.Error(), "file exists") {
		return nil, err
	}

	return &logStore{
		logs: &safeMap{
			make(map[uint64]*Log),
			sync.RWMutex{},
		},
		index:      1,
		term:       0,
		threshold:  2000,
		persistMux: &sync.Mutex{},
		piping:     false,
	}, nil
}

func (l *logStore) AppendLog(log *Log) error {
	if l.logs == nil {
		return fmt.Errorf("missing slice")
	}

	l.logs.Set(log.Index, log)

	go l.persistLog()
	return nil
}

func (l *logStore) SetLog(index uint64, log *Log) error {
	if l.logs == nil {
		return fmt.Errorf("missing slice")
	}

	l.logs.Set(index, log)
	return nil
}

func (l *logStore) GetLog(index uint64) (*Log, error) {
	if l.logs == nil {
		return nil, fmt.Errorf("missing slice")
	}

	log, ok := l.logs.Get(index)
	if ok {
		return log, nil
	}

	l.persistMux.Lock()
	defer l.persistMux.Unlock()

	// go to disk
	entries, err := os.ReadDir(LOG_DIR)
	if err != nil {
		return nil, err
	}

	name := ""
	for _, entry := range entries {
		splitEntry := strings.Split(entry.Name(), HYPEN)
		if len(splitEntry) < 2 {
			return nil, ErrLogFormatWrong
		}

		l, err := strconv.ParseUint(splitEntry[0], 10, 64)
		if err != nil {
			return nil, err
		}

		h, err := strconv.ParseUint(splitEntry[1], 10, 64)
		if err != nil {
			return nil, err
		}

		if h > index && index >= l {
			name = LOG_DIR + "/" + entry.Name()
			break
		}
	}

	if len(name) == 0 {
		return nil, fmt.Errorf("index missing: %v", index)
	}

	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if err := l.restoreLogs(f, name); err != nil {
		return nil, err
	}

	if log, ok := l.logs.Get(index); ok {
		return log, nil
	}

	return nil, fmt.Errorf("cannot find log: %v", index)
}

func (l *logStore) restoreLogs(rClose io.ReadCloser, name string) error {
	bts := []byte{}
	buffer := make([]byte, 1024)

	for {
		n, err := rClose.Read(buffer)
		if err != nil && err != io.EOF {
			return err
		}

		if n == 0 {
			break
		}

		bts = append(bts, buffer...)

		// TODO: find a better way that does not require two traverisals
		indexes := findIndexes(bts, BREAK_SYMBOL)
		if len(indexes) == 0 {
			continue
		}

		previous := 0

		for i := 0; i < len(indexes); i++ {
			subSlice := bts[previous:indexes[i]]
			if len(subSlice) == 0 {
				continue
			}

			if err := l.extractLog(&subSlice); err == nil {
				previous = indexes[i] + len(BREAK_SYMBOL)
			}
		}

		bts = bts[previous:]
	}

	return nil
}

func (l *logStore) extractLog(by *[]byte) error {
	payload := Log{}
	if err := msgpack.Unmarshal(*by, &payload); err != nil {
		return err
	}

	l.logs.Set(payload.Index, &payload)
	return nil
}

func (l *logStore) UpdateCommited(index uint64) (bool, error) {
	if l.logs == nil {
		return false, fmt.Errorf("missing slice")
	}

	log, ok := l.logs.Get(index)
	if !ok {
		return false, fmt.Errorf("cannot find log: %v", index)
	}

	log.LeaderCommited = true

	// TODO: breaks seperation/AppendLog side effect introduced so fix!
	// Also very brittle
	if log.Index-1 != l.index && !l.piping {
		fmt.Println("missing log!", log.Index, l.index)
		l.piping = true
		return true, nil
	}

	return false, nil
}

// writes the current batch of logs to disk
func (l *logStore) persistLog() error {
	l.persistMux.Lock()
	defer l.persistMux.Unlock()

	l.currBatch++

	// escape early if not ready to be push to cache
	if l.currBatch < l.threshold {
		return nil
	}

	l.currBatch = 0

	nextStart := uint64(0)

	entries, err := os.ReadDir(LOG_DIR)
	if err != nil {
		fmt.Println("unable to read:", err)
		return err
	}

	for _, entry := range entries {
		splitEntry := strings.Split(entry.Name(), HYPEN)
		if len(splitEntry) < 2 {
			fmt.Println("failed to split")
			return fmt.Errorf("unable to split")
		}

		i, err := strconv.ParseUint(splitEntry[1], 10, 64)
		if err != nil {
			fmt.Println("failed to parse")
			return err
		}

		if nextStart < i {
			nextStart = i
		}
	}

	// name of file
	nextStart += 1
	last := l.index - 5

	// happens when node is restarting
	if nextStart >= last {
		return nil
	}

	for k := nextStart; k <= last; k += l.threshold {
		lower := k
		upper := k + l.threshold
		if upper > last {
			upper = last
		} else {
			upper--
		}

		if upper-lower < 5 {
			break
		}

		f, err := os.Create(fmt.Sprintf(FILE_FORMAT, lower, upper))
		if err != nil {
			fmt.Println("failed to create file")
			return nil
		}
		defer f.Close()

		for i := lower; i <= upper; i++ {
			log, err := l.GetLog(i)
			if err != nil {
				fmt.Println("failed to get log when persisting:", err)
				return err
			}

			logData, err := msgpack.Marshal(log)
			if err != nil {
				fmt.Println("failed to marshall")
				return err
			}

			logData = append(logData, BREAK_SYMBOL...)
			if _, err := f.Write(logData); err != nil {
				fmt.Println("failed to write")
				return err
			}
		}

		// delete logs after
		l.deleteRange(0, upper)
	}

	return nil
}

func (l *logStore) RestoreLogs(app ApplicationApply) error {
	dir, err := os.ReadDir(LOG_DIR)
	if err != nil {
		fmt.Println("Error:", err)
		return err
	}

	var entries DirEntries
	for _, entry := range dir {
		entries = append(entries, entry)
	}

	sort.Sort(entries)

	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			// Skip non-regular files
			continue
		}

		file, err := os.Open(LOG_DIR + "/" + entry.Name())
		if err != nil {
			fmt.Println("Error:", err)
			continue
		}
		defer file.Close()

		if err := l.restore(app, io.ReadCloser(file)); err != nil {
			return err
		}
	}

	return nil
}

func (l *logStore) restore(app ApplicationApply, rClose io.ReadCloser) error {
	bts := []byte{}
	buffer := make([]byte, 1024)

	for {
		n, err := rClose.Read(buffer)
		if err != nil && err != io.EOF {
			return err
		}

		if n == 0 {
			break
		}

		bts = append(bts, buffer...)

		// TODO: find a better way that does not require two traverisals
		indexes := findIndexes(bts, BREAK_SYMBOL)
		if len(indexes) == 0 {
			continue
		}

		previous := 0

		for i := 0; i < len(indexes); i++ {
			subSlice := bts[previous:indexes[i]]
			if len(subSlice) == 0 {
				continue
			}

			if err := l.extractApplyLog(app, &subSlice); err == nil {
				previous = indexes[i] + len(BREAK_SYMBOL)
			}
		}

		bts = bts[previous:]
	}

	return nil
}

func (l *logStore) extractApplyLog(app ApplicationApply, data *[]byte) error {
	log := Log{}
	if err := msgpack.Unmarshal(*data, &log); err != nil {
		return err
	}

	// update to latest term and index found in snapshot
	if log.Term > l.term {
		l.term = log.Term
	}

	if log.Index > l.index {
		l.index = log.Index
	}

	if _, err := app.Apply(log); err != nil {
		fmt.Println("log failed on application")
	}

	return nil
}

func (l *logStore) ApplyFrom(index uint64, app ApplicationApply) {
	for {
		log, ok := l.logs.Get(index)
		if !ok {
			l.index = index - 1
			l.piping = false
			return
		}

		// we ignore error as maybe intentional
		if log.LeaderCommited {
			app.Apply(*log)
		}

		index++
	}
}

func (l *logStore) IncrementIndex() {
	l.index++
}

func (l *logStore) IncrementTerm() {
	l.term++
}

func (l *logStore) GetLatestIndex() uint64 {
	return l.index
}

func (l *logStore) GetLatestTerm() uint64 {
	return l.term
}

func (l *logStore) deleteRange(start, finish uint64) {
	l.logs.DeleteRange(start, finish)
}

func (l *logStore) IsPiping() bool {
	return l.piping
}

func (l *logStore) SetPiping(isPiping bool) {
	l.piping = isPiping
}

func findIndexes(largerSlice, smallerSlice []byte) []int {
	indexes := make([]int, 0)

	for i := 0; i <= len(largerSlice)-len(smallerSlice); i++ {
		match := true
		for j := 0; j < len(smallerSlice); j++ {
			if largerSlice[i+j] != smallerSlice[j] {
				match = false
				break
			}
		}
		if match {
			indexes = append(indexes, i)
		}
	}

	return indexes
}
