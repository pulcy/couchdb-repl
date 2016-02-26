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
	"reflect"
	"strconv"
	"time"

	"github.com/giantswarm/retry-go"
	"github.com/juju/errgo"
	"github.com/rhinoman/couchdb-go"
)

const (
	replicatorDbName = "_replicator"
	roleReplicator   = "replicator"
	roleEditor       = "editor"
)

type ReplicatorDocument struct {
	Source       string  `json:"source"`
	Target       string  `json:"target"`
	CreateTarget bool    `json:"create_target,omitempty"`
	Continuous   bool    `json:"continuous,omitempty"`
	UserCtx      UserCtx `json:"user_ctx"`
}

type UserCtx struct {
	Name  string   `json:"name"`
	Roles []string `json:"roles"`
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
	adminAuth := couchdb.BasicAuth{Username: s.AdminUser.UserName, Password: s.AdminUser.Password}

	// Create replicator user (if needed)
	replicationRoles := []string{roleReplicator}
	if err := do(func() error {
		return s.ensureUser(s.ReplicatorUser, replicationRoles, conn, &adminAuth)
	}); err != nil {
		return maskAny(errgo.Notef(err, "failed to create replicator user '%s', on '%s': %s", s.ReplicatorUser.UserName, serverURL, err.Error()))
	}

	// Create editor user (if needed)
	editorRoles := []string{roleEditor}
	if err := do(func() error {
		return s.ensureUser(s.EditorUser, editorRoles, conn, &adminAuth)
	}); err != nil {
		return maskAny(errgo.Notef(err, "failed to create editor user '%s', on '%s': %s", s.EditorUser.UserName, serverURL, err.Error()))
	}

	// Connect to db
	replicatorAuth := couchdb.BasicAuth{Username: s.ReplicatorUser.UserName, Password: s.ReplicatorUser.Password}

	// Configure roles for _replicator database
	adminRoles := []string{roleReplicator}
	if err := do(func() error {
		return s.configureDatabaseRoles(nil, adminRoles, conn.SelectDB(replicatorDbName, &adminAuth))
	}); err != nil {
		return maskAny(err)
	}

	// Create replicator document for all servers, for all databases
	for _, sourceURL := range s.ServerURLs {
		if sourceURL.String() == serverURL.String() {
			// Do not replicate with myself
			continue
		}
		for _, dbName := range s.DatabaseNames {
			// Configure database roles
			memberRoles := []string{roleEditor}
			adminRoles := []string{roleReplicator, roleEditor}
			if err := do(func() error {
				return s.configureDatabaseRoles(memberRoles, adminRoles, conn.SelectDB(dbName, &adminAuth))
			}); err != nil {
				return maskAny(err)
			}

			authURL := sourceURL
			authURL.User = url.UserPassword(s.ReplicatorUser.UserName, s.ReplicatorUser.Password)
			authURL.Path = dbName
			replDoc := ReplicatorDocument{
				Source:     authURL.String(),
				Target:     dbName,
				Continuous: true,
				UserCtx: UserCtx{
					Name:  s.ReplicatorUser.UserName,
					Roles: []string{roleReplicator},
				},
			}
			id := createId(replDoc)

			replicatorDb := conn.SelectDB(replicatorDbName, &replicatorAuth)
			update := func() error {
				s.Logger.Info("Updating replication database")
				if err := s.updateOrCreate(replicatorDb, id, replDoc); err != nil {
					s.Logger.Errorf("updateOrCreate failed: %#v", err)
					return maskAny(err)
				}
				return nil
			}
			err = retry.Do(update,
				retry.MaxTries(5),
				retry.Sleep(time.Second*2),
				retry.Timeout(time.Minute),
			)
			if err != nil {
				return maskAny(errgo.Notef(err, "failed to setup replicator document for '%s', source '%s': %s", dbName, sourceURL, err.Error()))
			}
		}
	}

	return nil
}

// ensureUser ensures that the given user exists in the given database server.
func (s *service) ensureUser(user UserInfo, roles []string, conn *couchdb.Connection, adminAuth couchdb.Auth) error {
	var userDoc couchdb.UserRecord
	if _, err := conn.GetUser(user.UserName, &userDoc, adminAuth); err == nil {
		// user exists, check the roles
		s.Logger.Debugf("user '%s' already exists", user.UserName)
		for _, r := range roles {
			if _, err := conn.GrantRole(user.UserName, r, adminAuth); err != nil {
				s.Logger.Errorf("Failed to grant role '%s' to user '%s': %#v", r, user.UserName, err)
				return maskAny(err)
			}
		}
		return nil
	} else if isCouchNotFound(err) {
		// Replicator user not found
		s.Logger.Infof("Adding user '%s'", user.UserName)
		if _, err := conn.AddUser(user.UserName, user.Password, roles, adminAuth); err != nil {
			s.Logger.Errorf("Failed to add user '%s': %#v", user.UserName, err)
			return maskAny(err)
		}
		return nil
	} else {
		// Some other error
		return maskAny(err)
	}
}

// configureDatabaseRoles ensures that the given database has at least the given member and admin roles.
func (s *service) configureDatabaseRoles(memberRoles, adminRoles []string, db *couchdb.Database) error {
	for _, r := range memberRoles {
		if err := db.AddRole(r, false); err != nil {
			s.Logger.Errorf("Failed to add member role '%s' to db: %#v", r, err)
			return maskAny(err)
		}
	}
	for _, r := range adminRoles {
		if err := db.AddRole(r, true); err != nil {
			s.Logger.Errorf("Failed to add admin role '%s' to db: %#v", r, err)
			return maskAny(err)
		}
	}
	return nil
}

func (s *service) updateOrCreate(db *couchdb.Database, id string, document ReplicatorDocument) error {
	var oldDoc ReplicatorDocument
	rev, err := db.Read(id, &oldDoc, nil)
	if isCouchNotFound(err) {
		// Not found, create new document
		rev = ""
	} else if err != nil {
		return maskAny(err)
	} else {
		// Compare document
		if reflect.DeepEqual(oldDoc, document) {
			// Nothing has changed
			s.Logger.Infof("nothing has changed in replicator-document '%s'", id)
			return nil
		}
	}

	// Remove the old document (if needed)
	if rev != "" {
		if _, err := db.Delete(id, rev); err != nil {
			return maskAny(err)
		}
	}

	// Save as new document
	if _, err := db.Save(document, id, ""); err != nil {
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

// do executes the given functions, retrying a few times when it fails.
func do(action func() error) error {
	if err := retry.Do(action,
		retry.MaxTries(5),
		retry.Sleep(time.Second*2),
		retry.Timeout(time.Minute),
	); err != nil {
		return maskAny(err)
	}
	return nil
}
