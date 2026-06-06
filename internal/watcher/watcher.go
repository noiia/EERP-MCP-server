// Package watcher monitors the docs directory and mkdocs.yml for changes,
// triggering a re-index via a callback.
package watcher

import (
	"io/fs"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

// ReindexFunc is called whenever a relevant file change is detected.
type ReindexFunc func() error

// Watcher watches a docs directory and a mkdocs.yml file for changes.
type Watcher struct {
	docsRoot   string
	mkdocsPath string
	reindex    ReindexFunc
	logger     *slog.Logger
	debounce   time.Duration
}

// New creates a Watcher. debounce controls how long to wait after the last
// change event before calling reindex (prevents thrashing during bulk saves).
func New(docsRoot, mkdocsPath string, reindex ReindexFunc, logger *slog.Logger) *Watcher {
	return &Watcher{
		docsRoot:   docsRoot,
		mkdocsPath: mkdocsPath,
		reindex:    reindex,
		logger:     logger,
		debounce:   500 * time.Millisecond,
	}
}

// Run starts the watch loop and blocks until ctx is done or a fatal error occurs.
func (w *Watcher) Run(stop <-chan struct{}) error {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer fw.Close()

	if err := fw.Add(w.docsRoot); err != nil {
		w.logger.Warn("cannot watch docs root", "path", w.docsRoot, "error", err)
	}
	if err := fw.Add(w.mkdocsPath); err != nil {
		w.logger.Warn("cannot watch mkdocs.yml", "path", w.mkdocsPath, "error", err)
	}

	// Also watch sub-directories that already exist.
	w.addSubdirs(fw, w.docsRoot)

	w.logger.Info("file watcher started", "docs", w.docsRoot, "mkdocs", w.mkdocsPath)

	timer := time.NewTimer(0)
	timer.Stop()
	pending := false

	for {
		select {
		case <-stop:
			return nil

		case event, ok := <-fw.Events:
			if !ok {
				return nil
			}
			if !isRelevant(event) {
				continue
			}
			w.logger.Debug("file changed", "path", event.Name, "op", event.Op)

			// Watch newly created directories.
			if event.Op&fsnotify.Create != 0 {
				w.addSubdirs(fw, event.Name)
			}

			if !pending {
				timer.Reset(w.debounce)
				pending = true
			}

		case err, ok := <-fw.Errors:
			if !ok {
				return nil
			}
			w.logger.Warn("watcher error", "error", err)

		case <-timer.C:
			pending = false
			w.logger.Info("reindexing due to file change")
			if err := w.reindex(); err != nil {
				w.logger.Error("reindex failed", "error", err)
			}
		}
	}
}

func (w *Watcher) addSubdirs(fw *fsnotify.Watcher, root string) {
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		_ = fw.Add(path)
		return nil
	})
}

// isRelevant returns true for write/create/remove/rename on .md and .yml files.
func isRelevant(e fsnotify.Event) bool {
	if e.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) == 0 {
		return false
	}
	ext := filepath.Ext(e.Name)
	return ext == ".md" || ext == ".yml" || ext == ".yaml"
}
