// Copyright 2023 Outreach Corporation. All Rights Reserved.

// Description: This file contains stubs for unit tests.
package e2e

import (
	"os"
	"time"
)

// StubFileInfo implements os.FileInfo and os.DirEntry
type StubFileInfo struct {
	FileName    string
	IsDirectory bool
}

func (mfi StubFileInfo) Name() string       { return mfi.FileName }
func (mfi StubFileInfo) Size() int64        { return int64(8) }
func (mfi StubFileInfo) Mode() os.FileMode  { return os.ModePerm }
func (mfi StubFileInfo) ModTime() time.Time { return time.Now() }
func (mfi StubFileInfo) IsDir() bool        { return mfi.IsDirectory }
func (mfi StubFileInfo) Sys() interface{}   { return nil }
func (mfi StubFileInfo) Type() os.FileMode  { return os.ModePerm }
func (mfi StubFileInfo) Info() (os.FileInfo, error) {
	return mfi, nil
}
