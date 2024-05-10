package raft

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"
)

type binaryStore struct {
	logs   *safeMap
	index  uint64
	term   uint64
	piping bool

	threshold  uint64
	currBatch  uint64
	persistMux *sync.Mutex
}

const (
	currentVersion uint8 = 1
)

func NewBinaryLogStore(threshold uint64) (LogStore, error) {
	// log directory - Create a folder/directory at a full qualified path
	if err := os.MkdirAll(LOG_DIR, 0755); err != nil && !strings.Contains(err.Error(), "file exists") {
		return nil, err
	}

	return &binaryStore{
		logs: &safeMap{
			make(map[uint64]*Log),
			sync.RWMutex{},
		},
		index:      1,
		term:       0,
		threshold:  threshold,
		persistMux: &sync.Mutex{},
		piping:     false,
	}, nil
}

func (l *binaryStore) AppendLog(log *Log) error {
	if l.logs == nil {
		return fmt.Errorf("missing slice")
	}

	l.logs.Set(log.Index, log)

	go l.persistLog()
	return nil
}

func (l *binaryStore) SetLog(index uint64, log *Log) error {
	if l.logs == nil {
		return fmt.Errorf("missing slice")
	}

	l.logs.Set(index, log)
	return nil
}

func (l *binaryStore) GetLog(index uint64) (*Log, error) {
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

	var dirEntries DirEntries
	for _, entry := range entries {
		dirEntries = append(entries, entry)
	}

	sort.Sort(dirEntries)

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

		if index >= l && index <= h {
			name = LOG_DIR + "/" + entry.Name()
			break
		}
	}

	if len(name) == 0 {
		return nil, fmt.Errorf("index missing: %v", index)
	}

	file, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	logs, err := l.readLogs(file)
	if err != nil {
		return nil, err
	}

	for _, lg := range logs {
		l.logs.Set(lg.Index, lg)
	}

	if log, ok := l.logs.Get(index); ok {
		return log, nil
	}

	return nil, fmt.Errorf("cannot find log: %v", index)
}

func (l *binaryStore) UpdateCommited(index uint64) (bool, error) {
	if l.logs == nil {
		return false, fmt.Errorf("missing slice")
	}

	lg, ok := l.logs.Get(index)
	if !ok {
		return false, fmt.Errorf("cannot find log: %v", index)
	}

	lg.LeaderCommited = true

	// TODO: breaks seperation/AppendLog side effect introduced so fix!
	// Also very brittle
	if lg.Index-1 != l.index && !l.piping {
		log.Info().Msg("missing a log, piping required")
		l.piping = true
		return true, nil
	}

	return false, nil
}

// writes the current batch of logs to disk
func (l *binaryStore) persistLog() error {
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
		log.Error().Err(err).Msg("unable to read snapshot file")
		return err
	}

	// TODO: Keep track of the last found instead of keep reading the files!!
	for _, entry := range entries {
		splitEntry := strings.Split(entry.Name(), HYPEN)
		if len(splitEntry) < 2 {
			log.Error().Str("filename", entry.Name()).Msg("failed to split file")
			return fmt.Errorf("unable to split")
		}

		i, err := strconv.ParseUint(splitEntry[1], 10, 64)
		if err != nil {
			log.Error().Str("filename", splitEntry[1]).Err(err).Msg("failed to parse uint64")
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

		fileLocation := fmt.Sprintf(FILE_FORMAT, lower, upper)
		f, err := os.Create(fileLocation)
		if err != nil {
			log.Error().Str("fileLocation", fileLocation).Err(err).Msg("failed to create file")
			return nil
		}

		if err := l.writeLogs(fileLocation, 1024, 1000, lower, upper); err != nil {
			log.Error().Err(err).Msg("failed to write logs to file")
			return err
		}

		f.Close()
		// delete logs after
		l.deleteRange(0, upper)
	}

	return nil
}

func (l *binaryStore) RestoreLogs(app ApplicationApply) error {
	dir, err := os.ReadDir(LOG_DIR)
	if err != nil {
		log.Error().Err(err).Msg("unable to read directory when restoring logs")
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

		location := LOG_DIR + "/" + entry.Name()
		file, err := os.Open(location)
		if err != nil {
			return err
		}
		defer file.Close()

		logs, err := l.readLogs(file)
		if err != nil {
			return err
		}

		for _, lg := range logs {
			// update to latest term and index found in snapshot
			if lg.Term > l.term {
				l.term = lg.Term
			}

			if lg.Index > l.index {
				l.index = lg.Index
			}

			if _, err := app.Apply(*lg); err != nil {
				log.Error().Uint64("log", lg.Index).Err(err).Msg("unable to apply log")
				return err
			}
		}

	}

	return nil
}

func (l *binaryStore) ApplyFrom(index uint64, app ApplicationApply) {
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

func (l *binaryStore) IncrementIndex() {
	l.index++
}

func (l *binaryStore) IncrementTerm() {
	l.term++
}

func (l *binaryStore) GetLatestIndex() uint64 {
	return l.index
}

func (l *binaryStore) GetLatestTerm() uint64 {
	return l.term
}

func (l *binaryStore) deleteRange(start, finish uint64) {
	l.logs.DeleteRange(start, finish)
}

func (l *binaryStore) IsPiping() bool {
	return l.piping
}

func (l *binaryStore) SetPiping(isPiping bool) {
	l.piping = isPiping
}

func (l *binaryStore) writeLogs(filename string, bufferSize int, batchSize, lower, upper uint64) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	// Create a buffer for serializing log entries
	buffer := bytes.NewBuffer(make([]byte, 0, bufferSize))

	for i := lower; i <= upper; i++ {
		lg, ok := l.logs.Get(i)
		if !ok {
			log.Error().Uint64("index", i).Msg("failed to get log when persisting")
			return err
		}

		// Serialize log entry
		// Write version
		if err := binary.Write(buffer, binary.BigEndian, currentVersion); err != nil {
			return err
		}

		if err := binary.Write(buffer, binary.BigEndian, lg.Term); err != nil {
			return err
		}
		if err := binary.Write(buffer, binary.BigEndian, lg.Index); err != nil {
			return err
		}
		if err := binary.Write(buffer, binary.BigEndian, lg.LogType); err != nil {
			return err
		}
		if err := binary.Write(buffer, binary.BigEndian, uint64(len(lg.Data))); err != nil {
			return err
		}

		if _, err := buffer.Write(lg.Data); err != nil {
			return err
		}
		if err := binary.Write(buffer, binary.BigEndian, lg.LeaderCommited); err != nil {
			return err
		}

		// Write to file if batch size is reached or it's the last log entry
		if (i-lower+1)%batchSize == 0 || i == upper {
			// Write the valid portion of the buffer to the file
			if _, err := file.Write(buffer.Bytes()); err != nil {
				return err
			}

			// Reset the buffer for reuse
			buffer.Reset()
		}
	}

	return nil
}

func (l *binaryStore) readLogs(file *os.File) ([]*Log, error) {
	logs := []*Log{}

	for {
		// Read version
		var version uint8
		if err := binary.Read(file, binary.BigEndian, &version); err != nil {
			if err == io.EOF {
				break // End of file
			}
			return nil, err
		}

		log := &Log{}

		// Read log data
		if err := binary.Read(file, binary.BigEndian, &log.Term); err != nil {
			return nil, err
		}
		if err := binary.Read(file, binary.BigEndian, &log.Index); err != nil {
			return nil, err
		}
		if err := binary.Read(file, binary.BigEndian, &log.LogType); err != nil {
			return nil, err
		}
		var dataLength uint64
		if err := binary.Read(file, binary.BigEndian, &dataLength); err != nil {
			return nil, err
		}
		log.Data = make([]byte, dataLength)
		if _, err := file.Read(log.Data); err != nil {
			return nil, err
		}
		if err := binary.Read(file, binary.BigEndian, &log.LeaderCommited); err != nil {
			return nil, err
		}

		logs = append(logs, log)
	}

	return logs, nil
}
