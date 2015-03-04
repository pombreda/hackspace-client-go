// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package isolate implements the code to process '.isolate' files.
package isolate

//
// Types for storage
//

type UploadItem struct {
	Digest           string
	Size             int64
	HighPriority     bool
	CompressionLevel int
}

type FileToUpload struct {
	UploadItem
	Path string
}

func (fa *FileAsset) ToUpload() FileToUpload {
	// TODO(tandrii): get_zip_compression_level.
	return FileToUpload{
		UploadItem: UploadItem{
			Digest:           fa.meta["h"],
			Size:             fa.GetSize(),
			HighPriority:     fa.IsHighPriority(),
			CompressionLevel: 6,
		},
		Path: fa.fullPath,
	}
}

//
// STORAGE & Isolate Server API
//

type Storage struct {
	api StorageApier
}

func (storage *Storage) Connect() error {
	return nil
}

func (storage *Storage) Upload(done <-chan struct{}, chItems <-chan FileToUpload) error {
	// TODO
	return nil
}

type StorageApier interface {
	// TODO api
}

func GetStorageApi(server, namespace string) StorageApier {
	// TODO(tandrii):
	return StorageApiLog{server, namespace}
}

type StorageApiLog struct {
	server, namespace string
	// TODO dummy API
}
