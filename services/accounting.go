package services

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/inconshreveable/log15"
	dto "github.com/prometheus/client_model/go"
	"github.com/stephane-martin/skewer/accounting"
	"github.com/stephane-martin/skewer/conf"
	"github.com/stephane-martin/skewer/model"
	"github.com/stephane-martin/skewer/services/base"
	"github.com/stephane-martin/skewer/utils"
)

func initAccountingRegistry() {
	base.Once.Do(func() {
		base.InitRegistry()
	})
}

type AccountingService struct {
	stasher        base.Stasher
	logger         log15.Logger
	wgroup         *sync.WaitGroup
	Conf           conf.AccountingConfig
	stopchan       chan struct{}
	fatalErrorChan chan struct{}
	fatalOnce      *sync.Once
	confined       bool
}

func NewAccountingService(env *base.ProviderEnv) (base.Provider, error) {
	initAccountingRegistry()
	s := AccountingService{
		stasher:  env.Reporter,
		logger:   env.Logger.New("class", "accounting"),
		wgroup:   &sync.WaitGroup{},
		confined: env.Confined,
	}
	return &s, nil
}

func (s *AccountingService) Type() base.Types {
	return base.Accounting
}

func (s *AccountingService) Gather() ([]*dto.MetricFamily, error) {
	return base.Registry.Gather()
}

func readFileUntilEnd(f *os.File, size int) (err error) {
	// read the acct file until the end
	buf := make([]byte, accounting.Ssize)
	for {
		_, err = io.ReadAtLeast(f, buf, size)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return nil
		} else if err != nil {
			return fmt.Errorf("Unexpected error while reading the accounting file: %s", err)
		}
	}
}

func (s *AccountingService) makeMessage(buf []byte, tick int64, hostname string, gen *utils.Generator) model.FullMessage {
	acct := accounting.MakeAcct(buf, tick)
	props := acct.Properties()
	return model.FullMessage{
		ConfId: s.Conf.ConfID,
		Uid:    gen.Uid(),
		Parsed: model.ParsedMessage{
			Client:         hostname,
			LocalPort:      0,
			UnixSocketPath: "",
			Fields: model.SyslogMessage{
				Appname:          "accounting",
				Facility:         0,
				Hostname:         hostname,
				Msgid:            "",
				Priority:         0,
				Procid:           props["pid"],
				Severity:         0,
				Properties:       map[string]map[string]string{"acct": acct.Properties()},
				Structured:       "",
				TimeGeneratedNum: acct.Btime.UnixNano(),
				TimeReportedNum:  time.Now().UnixNano(),
				Version:          0,
				Message:          fmt.Sprintf("Accounting: %s (%s/%s)", props["comm"], props["uid"], props["gid"]),
			},
		},
	}
}

var ErrTruncated error = errors.New("File has been truncated")

func (s *AccountingService) readFile(f *os.File, tick int64, hostname string, size int) (err error) {
	var offset int64
	var fsize int64
	var infos os.FileInfo
	buf := make([]byte, accounting.Ssize)
	gen := utils.NewGenerator()
	for {
		_, err = io.ReadAtLeast(f, buf, size)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			// check if file has been truncated
			offset, err = f.Seek(0, 1)
			if err != nil {
				return err
			}
			infos, err = f.Stat()
			if err != nil {
				return err
			}
			fsize = infos.Size()
			if offset > fsize {
				s.logger.Info("Accounting file has been truncated", "offset", offset, "filesize", fsize)
				return ErrTruncated
			}
			return nil
		} else if err != nil {
			return fmt.Errorf("Unexpected error while reading the accounting file: %s", err)
		} else {
			f, nf := s.stasher.Stash(s.makeMessage(buf, tick, hostname, gen))
			if nf != nil {
				s.logger.Warn("Non-fatal error stashing accounting message", "error", nf)
			} else if f != nil {
				s.logger.Error("Fatal error stashing accounting message", "error", f)
				return f
			} else {
				base.IncomingMsgsCounter.WithLabelValues("accounting", hostname, "", "").Inc()
			}
		}
	}
}

func (s *AccountingService) doStart(watcher *fsnotify.Watcher, hostname string, f *os.File, tick int64) {
	defer func() {
		_ = f.Close()
		s.wgroup.Done()
	}()
	var err error

	err = watcher.Add(s.Conf.Path)
	if err != nil {
		s.logger.Error("Error starting to watch accounting file")
		return
	}

Read:
	// fetch content from the acct file
	for {
		err = s.readFile(f, tick, hostname, accounting.Ssize)
		if err == ErrTruncated {
			// file truncation was detected
			_, err = f.Seek(0, 0)
			if err != nil {
				s.logger.Error("Error when seeking to the beginning of the accounting file", "error", err)
				_ = watcher.Close()
				s.dofatal()
				return
			}
			continue Read
		} else if err != nil {
			s.logger.Error("Error reading the accounting file", "error")
			_ = watcher.Close()
			s.dofatal()
			return
		}

	WaitWrite:
		for {
			select {
			case err := <-watcher.Errors:
				s.logger.Warn("Watcher error", "error", err)
			case ev := <-watcher.Events:
				switch ev.Op {
				case fsnotify.Write:
					break WaitWrite
				case fsnotify.Rename:
					// accounting file rotation
					s.logger.Info("Accounting file has been renamed (rotation?)", "notifypath", ev.Name)
					time.Sleep(3 * time.Second)
					f2, err := os.Open(s.Conf.Path)
					if err == nil {
						s.logger.Info("Accounting file has been reopened", "path", s.Conf.Path)
					} else {
						s.logger.Error("Error reopening accounting file", "error", err, "path", s.Conf.Path)
						_ = watcher.Close()
						s.dofatal()
						return
					}
					s.wgroup.Add(1)
					go s.doStart(watcher, hostname, f2, tick)
					return
				case fsnotify.Remove:
					s.logger.Error("Accounting file has been removed ?!", "notifypath", ev.Name)
					_ = watcher.Close()
					s.dofatal()
					return
				default:
				}
			case <-s.stopchan:
				_ = watcher.Close()
				return
			}
		}

	}

}

func (s *AccountingService) FatalError() chan struct{} {
	return s.fatalErrorChan
}

func (s *AccountingService) dofatal() {
	s.fatalOnce.Do(func() { close(s.fatalErrorChan) })
}

func (s *AccountingService) Start() (infos []model.ListenerInfo, err error) {
	infos = []model.ListenerInfo{}
	s.stopchan = make(chan struct{})
	s.fatalErrorChan = make(chan struct{})
	s.fatalOnce = &sync.Once{}
	tick := accounting.Tick()
	var f *os.File

	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	acctFilename := s.Conf.Path
	if s.confined {
		acctFilename = filepath.Join("/tmp", "acct", acctFilename)
	}
	f, err = os.Open(acctFilename)
	if err != nil {
		return
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return
	}

	s.wgroup.Add(1)
	go func() {
		defer s.wgroup.Done()
		err = readFileUntilEnd(f, accounting.Ssize)
		if err != nil {
			s.logger.Error("Error reading the accounting file for the first time", "error", err)
			_ = watcher.Close()
			s.dofatal()
			return
		}
		s.wgroup.Add(1)
		go s.doStart(watcher, hostname, f, tick)
	}()
	return
}

func (s *AccountingService) Stop() {
	if s.stopchan != nil {
		close(s.stopchan)
		s.stopchan = nil
	}
	s.wgroup.Wait()
}

func (s *AccountingService) Shutdown() {
	if s.stopchan != nil {
		close(s.stopchan)
		s.stopchan = nil
	}
	s.wgroup.Wait()
}

func (s *AccountingService) SetConf(c conf.BaseConfig) {
	s.Conf = c.Accounting
}
