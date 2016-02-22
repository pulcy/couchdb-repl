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
	"net/url"
	"time"

	"github.com/op/go-logging"
	"github.com/rhinoman/couchdb-go"
)

const (
	couchdbTimeout = time.Millisecond * 500
)

type ServiceConfig struct {
	ServerURLs    []url.URL
	UserName      string
	Password      string
	DatabaseNames []string
}

type ServiceDependencies struct {
	Logger *logging.Logger
}
type service struct {
	ServiceConfig
	ServiceDependencies

	conn *couchdb.Connection
}

func NewService(config ServiceConfig, deps ServiceDependencies) *service {
	return &service{
		ServiceConfig:       config,
		ServiceDependencies: deps,
	}
}

// Run performs a setup of the replicator databases
func (s *service) Run() error {
	for _, url := range s.ServerURLs {
		s.Logger.Info("Configuring replication for '%s'", url)
		if err := s.setupReplication(url); err != nil {
			s.Logger.Info("Configuring replication for '%s' failed: %#v", url, err)
			return maskAny(err)
		}
	}
	return nil
}
