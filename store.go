package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/boltdb/bolt"
)

var (
	linkBucket    = []byte("links")
	articleBucket = []byte("articles")
)

var errNotFound = errors.New("not found")

type store struct {
	db *bolt.DB
}

type articleState struct {
	ID          int64
	URL         string
	DismissedAt time.Time
}

type linkState struct {
	ID         int64
	URL        string
	ArticleURL string
	QueuedAt   time.Time
}

func newStore(path string) (*store, error) {
	db, err := bolt.Open(path, 0600, &bolt.Options{
		Timeout: 1 * time.Second,
	})
	if err != nil {
		return nil, err
	}

	// create initial buckets
	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(linkBucket)
		if err != nil {
			return fmt.Errorf("error creating bucket: %v", err)
		}

		_, err = tx.CreateBucketIfNotExists(articleBucket)
		if err != nil {
			return fmt.Errorf("error creating bucket: %v", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &store{db: db}, nil
}

func (s *store) findArticle(url string) (*articleState, error) {
	var article articleState
	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(articleBucket)
		buf := bucket.Get([]byte(url))
		if buf == nil {
			return errNotFound
		}
		return json.Unmarshal(buf, &article)
	})
	if err != nil {
		return nil, err
	}
	return &article, nil
}

func (s *store) markArticleDismissed(url string) error {
	buf, err := json.Marshal(articleState{
		URL:         url,
		DismissedAt: time.Now(),
	})
	if err != nil {
		return err
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(articleBucket)
		return bucket.Put([]byte(url), buf)
	})
}

func (s *store) findLink(url string) (*linkState, error) {
	var link linkState
	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(linkBucket)
		buf := bucket.Get([]byte(url))
		if buf == nil {
			return errNotFound
		}
		return json.Unmarshal(buf, &link)
	})
	if err != nil {
		return nil, err
	}
	return &link, nil
}

func (s *store) markLinkQueued(url string) error {
	buf, err := json.Marshal(linkState{
		URL:      url,
		QueuedAt: time.Now(),
	})
	if err != nil {
		return err
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(linkBucket)
		return bucket.Put([]byte(url), buf)
	})
}
