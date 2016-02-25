// Copyright (c) 2016 Pulcy.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package service

import (
	"crypto/sha1"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/giantswarm/retry-go"
	"github.com/juju/errgo"
	"github.com/rhinoman/couchdb-go"
)

const (
	replicatorDbName = "_replicator"
)

type ReplicatorDocument struct {
	Source       string `json:"source"`
	Target       string `json:"target"`
	CreateTarget bool   `json:"create_target,omitempty"`
	Continuous   bool   `json:"continuous,omitempty"`
}

func (s *service) setupReplication(serverURL url.URL) error {
	// Open couchDB connection to given URL
	host, port, err := net.SplitHostPort(serverURL.Host)
	if err != nil {
		return maskAny(err)
	}
	portNr, err := strconv.Atoi(port)
	if err != nil {
		return maskAny(err)
	}
	conn, err := couchdb.NewConnection(host, portNr, couchdbTimeout)
	if err != nil {
		return maskAny(errgo.Notef(err, "cannot create database connection: %s", err.Error()))
	}
	ping := func() error {
		return maskAny(conn.Ping())
	}
	err = retry.Do(ping,
		retry.MaxTries(60),
		retry.Sleep(time.Second*2),
		retry.Timeout(time.Minute*5),
	)
	if err != nil {
		return maskAny(errgo.Notef(err, "cannot ping database: %s", err.Error()))
	}

	// Connect to replicator database
	auth := couchdb.BasicAuth{Username: s.UserName, Password: s.Password}
	db := conn.SelectDB(replicatorDbName, &auth)

	// Create replicator document for all servers, for all databases
	for _, sourceURL := range s.ServerURLs {
		if sourceURL.String() == serverURL.String() {
			// Do not replicate with myself
			continue
		}
		for _, dbName := range s.DatabaseNames {
			authURL := sourceURL
			authURL.User = url.UserPassword(s.UserName, s.Password)
			authURL.Path = dbName
			replDoc := ReplicatorDocument{
				Source:     authURL.String(),
				Target:     dbName,
				Continuous: true,
			}
			id := createId(replDoc)

			update := func() error {
				return maskAny(updateOrCreate(db, id, replDoc))
			}
			err = retry.Do(update,
				retry.MaxTries(15),
				retry.Sleep(time.Second*2),
				retry.Timeout(time.Minute*5),
			)
			if err != nil {
				return maskAny(errgo.Notef(err, "failed to setup replicator document for '%s', source '%s': %s", dbName, sourceURL, err.Error()))
			}
		}
	}

	return nil
}

func updateOrCreate(db *couchdb.Database, id string, document ReplicatorDocument) error {
	var oldDoc ReplicatorDocument
	rev, err := db.Read(id, &oldDoc, nil)
	if isCouchNotFound(err) {
		// Not found, create new document
		rev = ""
	} else if err != nil {
		return maskAny(err)
	}

	if _, err := db.Save(document, id, rev); err != nil {
		return maskAny(err)
	}

	return nil
}

func isCouchNotFound(err error) bool {
	if cerr, ok := err.(*couchdb.Error); ok {
		return cerr.StatusCode == http.StatusNotFound
	}
	return false
}

func createId(replDoc ReplicatorDocument) string {
	data := fmt.Sprintf("%s,%s", replDoc.Source, replDoc.Target)
	return fmt.Sprintf("%x", sha1.Sum([]byte(data)))
}
