package execution

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type FileStore struct {
	Root string
}

func (store FileStore) Save(record Record) error {
	data, err := MarshalRecord(record)
	if err != nil {
		return fmt.Errorf("save operation record: %w", err)
	}
	if store.Root == "" || !operationIDPattern.MatchString(record.ID) {
		return errors.New("save operation record: invalid store root or operation ID")
	}
	if err := ensurePrivateDirectory(store.Root); err != nil {
		return err
	}
	destination := filepath.Join(store.Root, record.ID+".json")
	if err := rejectSymlink(destination); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(store.Root, ".operation-*.tmp")
	if err != nil {
		return fmt.Errorf("save operation record: create temporary file: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("save operation record: protect temporary file: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return fmt.Errorf("save operation record: write temporary file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("save operation record: sync temporary file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("save operation record: close temporary file: %w", err)
	}
	if err := replaceFile(temporaryName, destination); err != nil {
		return fmt.Errorf("save operation record: replace record: %w", err)
	}
	return nil
}

func (store FileStore) Load(id string) (Record, error) {
	if store.Root == "" || !operationIDPattern.MatchString(id) {
		return Record{}, errors.New("load operation record: invalid store root or operation ID")
	}
	path := filepath.Join(store.Root, id+".json")
	if err := rejectSymlink(path); err != nil {
		return Record{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		backup := path + ".bak"
		if !errors.Is(err, os.ErrNotExist) {
			return Record{}, fmt.Errorf("load operation record: %w", err)
		}
		if err := rejectSymlink(backup); err != nil {
			return Record{}, err
		}
		data, err = os.ReadFile(backup)
		if err != nil {
			return Record{}, fmt.Errorf("load operation record: %w", err)
		}
	}
	value, err := DecodeRecord(data)
	if err != nil {
		return Record{}, fmt.Errorf("load operation record: %w", err)
	}
	return value, nil
}

func ensurePrivateDirectory(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return fmt.Errorf("save operation record: create history directory: %w", err)
		}
		info, err = os.Lstat(path)
	}
	if err != nil {
		return fmt.Errorf("save operation record: inspect history directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("save operation record: history root must be a real directory")
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return fmt.Errorf("save operation record: protect history directory: %w", err)
	}
	return nil
}

func rejectSymlink(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("operation record: inspect path: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("operation record: destination must be a regular file")
	}
	return nil
}

func replaceFile(source, destination string) error {
	if err := os.Rename(source, destination); err == nil {
		return nil
	}
	backup := destination + ".bak"
	if err := rejectSymlink(backup); err != nil {
		return err
	}
	_ = os.Remove(backup)
	if err := os.Rename(destination, backup); err != nil {
		return err
	}
	if err := os.Rename(source, destination); err != nil {
		_ = os.Rename(backup, destination)
		return err
	}
	_ = os.Remove(backup)
	return nil
}
