// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package isolateserver

import "errors"

// ISOLATE_PROTOCOL_VERSION is passed to the serverUrl in /handshake request.
const ISOLATE_PROTOCOL_VERSION = "1.0"

// ALREADY_COMPRESSED_TYPES is a list of already compressed extension types
// that should not receive any compression before being uploaded.
var ALREADY_COMPRESSED_TYPES = []string{
	"7z", "avi", "cur", "gif", "h264", "jar", "jpeg", "jpg", "mp4", "pdf",
	"png", "wav", "zip",
}

type UploadItem interface {
	GetDigest() string
	GetSize() int64
	IsHighPriority() bool
	GetCompressionLevel() int
	GetContent(done <-chan struct{}) (<-chan []byte, <-chan error)
}

type Item struct {
	Digest           string
	Size             int64
	HighPriority     bool
	CompressionLevel int
}

func (i *Item) GetDigest() string {
	return i.Digest
}

func (i *Item) GetSize() int64 {
	return i.Size
}

func (i *Item) IsHighPriority() bool {
	return i.HighPriority
}

func (i *Item) GetCompressionLevel() int {
	return i.CompressionLevel
}

func (i *Item) GetContent(done <-chan struct{}) (<-chan []byte, <-chan error) {
	chError := make(chan error, 1)
	chError <- errors.New("not implemented for Item struct.")
	close(chError)
	return nil, chError
}

type FileItem struct {
	Item
	Path string
}

func (f *FileItem) GetContent(done <-chan struct{}) (<-chan []byte, <-chan error) {
	chOut := make(chan []byte)
	chError := make(chan error, 1)
	go func() {
		defer close(chOut)
		defer close(chError)
		// TODO(tandrii): read file
		chError <- errors.New("GetContent Not implemented")
	}()
	return chOut, chError
}

type PushState struct {
	// TODO
}

// StorageApi is an interface for classes that implement low-level storage operations.
// StorageApi is oblivious of compression and hashing scheme used. This details
// are handled in higher level Storage class.
//
// Clients should generally not use StorageApi directly. Storage class is
// preferred since it implements compression and upload optimizations.
type StorageApi interface {
	Location() string
	Namespace() string

	// GetFetchUrl returns an URL that can be used to fetch an item with given digest.
	//
	// Arguments:
	//  digest: hex digest of item to fetch.
	// Returns:
	//  A URL or error if the protocol doesn't support this.
	GetFetchUrl(digest string) (string, error)

	// Fetch an object and return its contents in chunks.
	//
	// Arguments:
	//  digest: hash digest of item to download.
	//  offset: offset (in bytes) from the start of the file to resume fetch from.
	// Returns:
	// 	A stream of chunks
	Fetch(done <-chan struct{}, digest string, offset int64) (<-chan []byte, <-chan error)

	// Push uploads an item.
	//
	// The item MUST go through Contains call to get PushState before it can
	// be pushed to the storage.
	//
	// To be clear, here is one possible usage:
	// TODO(tandrii): add usage
	//
	// When pushing to a namespace with compression, data that should be pushed
	// and data provided by the item is not the same. In that case |content| is
	// not None and it yields chunks of compressed data (using item.content() as
	// a source of original uncompressed data). This is implemented by Storage
	// class.
	//
	// Arguments:
	//  item: Item object that holds information about an item being pushed.
	//  push_state: push state object as returned by 'contains' call.
	Push(done <-chan struct{}, item UploadItem, pushState PushState) <-chan error

	// Checks for items on the server, prepares missing ones for upload.
	//
	// Arguments:
	//   items: list of UploadItem objects to check for presence.
	//
	// Returns:
	//   A dict missing Item -> opaque push state object to be passed to 'push'.
	//   See doc string for 'push'.
	Contains(items []UploadItem) map[UploadItem]PushState
}

func GetStorageApi(serverUrl, namespace string) StorageApi {
	// TODO(tandrii): add real API next.
	d := DryLoggingStorageApi{serverUrl, namespace, make([][]interface{}, 0)}
	return &d
}

// DryLoggingStorageApi doesn't actually do anything but logs every api call.
type DryLoggingStorageApi struct {
	serverUrl, namespace string
	events               [][]interface{}
}

func (a *DryLoggingStorageApi) Location() string {
	return a.serverUrl
}
func (a *DryLoggingStorageApi) Namespace() string {
	return a.namespace
}
func (a *DryLoggingStorageApi) GetFetchUrl(string) (string, error) {
	return "", errors.New("not implemented for DryLoggingStorageApi")
}

func (a *DryLoggingStorageApi) Fetch(<-chan struct{}, string, int64) (<-chan []byte, <-chan error) {
	return nil, nil
}

func (a *DryLoggingStorageApi) Push(done <-chan struct{}, item UploadItem, pushState PushState) <-chan error {
	return nil
}

func (a *DryLoggingStorageApi) Contains(items []UploadItem) map[UploadItem]PushState {
	return nil
}

type Storage struct {
	api StorageApi
	// TODO(tandrii): maybe add hashalgo support for namespaces.
	useCompression bool
}

func NewStorage(serverUrl, namespace string) Storage {
	return Storage{
		GetStorageApi(serverUrl, namespace),
		false, //TODO
	}
}

func (s *Storage) Connect() error {
	return nil
}

func (s *Storage) Upload(done <-chan struct{}, chItems <-chan UploadItem) error {
	// TODO
	return nil
}
