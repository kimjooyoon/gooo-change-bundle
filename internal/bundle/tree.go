package bundle

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

type treeSnapshot struct {
	Manifest TreeManifest
	Files    map[string][]byte
}

type treeError struct {
	Code string
	Path string
	Err  error
}

func (e *treeError) Error() string {
	if e.Path == "" {
		return e.Code + ": " + e.Err.Error()
	}
	return fmt.Sprintf("%s at %s: %v", e.Code, e.Path, e.Err)
}

func collectTree(root string) (treeSnapshot, error) {
	absRoot, err := absolutePath(root)
	if err != nil {
		return treeSnapshot{}, &treeError{Code: "SOURCE_TREE_UNOBSERVABLE", Err: err}
	}
	info, err := os.Lstat(absRoot)
	if err != nil {
		return treeSnapshot{}, &treeError{Code: "SOURCE_TREE_UNOBSERVABLE", Err: err}
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return treeSnapshot{}, &treeError{Code: "SYMLINK_ESCAPE", Path: ".", Err: fmt.Errorf("source root is a symlink")}
	}
	if !info.IsDir() {
		return treeSnapshot{}, &treeError{Code: "SOURCE_TREE_UNOBSERVABLE", Err: fmt.Errorf("source root is not a directory")}
	}

	snapshot := treeSnapshot{Manifest: TreeManifest{Schema: TreeManifestSchema, RootPolicy: "NO_SYMLINKS; EXCLUDE_EXACT_ROOT_.git"}, Files: make(map[string][]byte)}
	err = filepath.WalkDir(absRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			rel, _ := filepath.Rel(absRoot, path)
			return &treeError{Code: "SOURCE_TREE_UNOBSERVABLE", Path: filepath.ToSlash(rel), Err: walkErr}
		}
		if path == absRoot {
			return nil
		}
		rel, relErr := filepath.Rel(absRoot, path)
		if relErr != nil {
			return &treeError{Code: "SOURCE_TREE_UNOBSERVABLE", Err: relErr}
		}
		rel = filepath.ToSlash(rel)
		if rel == ".git" {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return &treeError{Code: "SYMLINK_ESCAPE", Path: rel, Err: fmt.Errorf("symlinks are not part of the source tree")}
		}
		if entry.IsDir() {
			snapshot.Manifest.Entries = append(snapshot.Manifest.Entries, TreeEntry{Path: rel, Kind: "directory"})
			return nil
		}
		if !entry.Type().IsRegular() {
			return &treeError{Code: "SOURCE_TREE_UNOBSERVABLE", Path: rel, Err: fmt.Errorf("unsupported filesystem entry")}
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return &treeError{Code: "SOURCE_TREE_UNOBSERVABLE", Path: rel, Err: readErr}
		}
		snapshot.Files[rel] = append([]byte(nil), data...)
		snapshot.Manifest.Entries = append(snapshot.Manifest.Entries, TreeEntry{Path: rel, Kind: "file", Bytes: len(data), Digest: DigestBytes(data)})
		return nil
	})
	if err != nil {
		return treeSnapshot{}, err
	}
	sort.Slice(snapshot.Manifest.Entries, func(i, j int) bool { return snapshot.Manifest.Entries[i].Path < snapshot.Manifest.Entries[j].Path })
	snapshot.Manifest.SourceDigest, err = treeDigest(snapshot.Manifest)
	if err != nil {
		return treeSnapshot{}, err
	}
	return snapshot, nil
}

func treeDigest(manifest TreeManifest) (string, error) {
	manifest.SourceDigest = ""
	return DigestValue(manifest)
}

func postimageSnapshot(base treeSnapshot, changes []ProposalChange) (treeSnapshot, error) {
	post := treeSnapshot{Manifest: base.Manifest, Files: make(map[string][]byte, len(base.Files))}
	post.Manifest.Entries = append([]TreeEntry(nil), base.Manifest.Entries...)
	for path, data := range base.Files {
		post.Files[path] = append([]byte(nil), data...)
	}
	entries := make(map[string]TreeEntry, len(base.Manifest.Entries))
	for _, entry := range base.Manifest.Entries {
		entries[entry.Path] = entry
	}
	for _, change := range changes {
		data, err := decodePostimage(change)
		if err != nil {
			return treeSnapshot{}, err
		}
		switch change.Operation {
		case OperationAdd, OperationModify:
			post.Files[change.Path] = data
			entries[change.Path] = TreeEntry{Path: change.Path, Kind: "file", Bytes: len(data), Digest: DigestBytes(data)}
		case OperationDelete:
			delete(post.Files, change.Path)
			delete(entries, change.Path)
		}
	}
	post.Manifest.Entries = post.Manifest.Entries[:0]
	for _, entry := range entries {
		post.Manifest.Entries = append(post.Manifest.Entries, entry)
	}
	sort.Slice(post.Manifest.Entries, func(i, j int) bool { return post.Manifest.Entries[i].Path < post.Manifest.Entries[j].Path })
	var err error
	post.Manifest.SourceDigest, err = treeDigest(post.Manifest)
	return post, err
}
