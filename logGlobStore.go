package raft

import (
	"encoding/gob"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"
)

type logGlobStore struct {
	logs   *safeMap
	index  uint64
	term   uint64
	piping bool

	threshold  uint64
	currBatch  uint64
	persistMux *sync.Mutex
}

func NewLogGlobStore(threshold uint64) (LogStore, error) {
	// log directory - Create a folder/directory at a full qualified path
	err := os.MkdirAll(LOG_DIR, 0755)
	if err != nil && !strings.Contains(err.Error(), "file exists") {
		return nil, err
	}

	return &logGlobStore{
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

func (l *logGlobStore) AppendLog(log *Log) error {
	if l.logs == nil {
		return fmt.Errorf("missing slice")
	}

	l.logs.Set(log.Index, log)

	go l.persistLog()
	return nil
}

func (l *logGlobStore) SetLog(index uint64, log *Log) error {
	if l.logs == nil {
		return fmt.Errorf("missing slice")
	}

	l.logs.Set(index, log)
	return nil
}

func (l *logGlobStore) GetLog(index uint64) (*Log, error) {
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

	logs, err := l.readLogsFromDisk(name)
	if err != nil {
		return nil, err
	}

	for _, lg := range *logs {
		l.logs.Set(lg.Index, lg)
	}

	if log, ok := l.logs.Get(index); ok {
		return log, nil
	}

	return nil, fmt.Errorf("cannot find log: %v", index)
}

func (l *logGlobStore) readLogsFromDisk(location string) (*[]*Log, error) {
	logs := []*Log{}
	file, err := os.Open(location)
	if err != nil {
		log.Error().Str("location", location).Err(err).Msg("unable to open file")
		return nil, err
	}

	defer file.Close()

	decoder := gob.NewDecoder(file)
	if err := decoder.Decode(&logs); err != nil {
		log.Error().Str("location", location).Err(err).Msg("unable to decode file")
		return nil, err
	}

	return &logs, nil
}

func (l *logGlobStore) UpdateCommited(index uint64) (bool, error) {
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
func (l *logGlobStore) persistLog() error {
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

		logs := []*Log{}
		for i := lower; i <= upper; i++ {
			lg, err := l.GetLog(i)
			if err != nil {
				log.Error().Uint64("index", i).Err(err).Msg("failed to get log when persisting")
				return err
			}

			logs = append(logs, lg)
		}

		fileLocation := fmt.Sprintf(FILE_FORMAT, lower, upper)
		f, err := os.Create(fileLocation)
		if err != nil {
			log.Error().Str("fileLocation", fileLocation).Err(err).Msg("failed to create file")
			return nil
		}

		encoder := gob.NewEncoder(f)
		if err := encoder.Encode(logs); err != nil {
			f.Close()
			// TODO: Delete file
			return err
		}

		f.Close()
		// delete logs after
		l.deleteRange(0, upper)
	}

	return nil
}

func (l *logGlobStore) RestoreLogs(app ApplicationApply) error {
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
		logs, err := l.readLogsFromDisk(location)
		if err != nil {
			return err
		}

		for _, lg := range *logs {
			// update to latest term and index found in snapshot
			if lg.Term > l.term {
				l.term = lg.Term
			}

			if lg.Index > l.index {
				l.index = lg.Index
			}

			if _, err := app.Apply(*lg); err != nil {
				log.Error().Uint64("log", lg.Index).Err(err).Msg("unable to apply log")
			}
		}

	}

	return nil
}

func (l *logGlobStore) ApplyFrom(index uint64, app ApplicationApply) {
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

func (l *logGlobStore) IncrementIndex() {
	l.index++
}

func (l *logGlobStore) IncrementTerm() {
	l.term++
}

func (l *logGlobStore) GetLatestIndex() uint64 {
	return l.index
}

func (l *logGlobStore) GetLatestTerm() uint64 {
	return l.term
}

func (l *logGlobStore) deleteRange(start, finish uint64) {
	l.logs.DeleteRange(start, finish)
}

func (l *logGlobStore) IsPiping() bool {
	return l.piping
}

func (l *logGlobStore) SetPiping(isPiping bool) {
	l.piping = isPiping
}
