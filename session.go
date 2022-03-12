// Copyright 2022 rsn Author. All Rights Reserved.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//      http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package rsn

import (
	"fmt"
	"os"
	"time"

	rds "github.com/go-redis/redis"

	se "github.com/go-the-way/anoweb/session"
)

type session struct {
	id          string
	key         string
	invalidated bool
	client      *rds.Client
}

func newSession(client *rds.Client, id, key string) se.Session {
	return &session{id, key, false, client}
}

const sessionIdName = "sessionId"

// Id return session id
func (s *session) Id() string {
	return s.id
}

// Renew session
func (s *session) Renew(lifeTime time.Duration) {
	s.client.Expire(s.key, lifeTime)
}

// Invalidated session
func (s *session) Invalidated() bool {
	return s.invalidated
}

// Invalidate session
func (s *session) Invalidate() {
	s.invalidated = true
}

// Get session named val
func (s *session) Get(name string) interface{} {
	getCmd := s.client.HGet(s.key, name)
	val := ""
	err := getCmd.Scan(&val)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
	}
	if val == "" {
		return nil
	}
	return val
}

// GetAll session's values
func (s *session) GetAll() map[string]interface{} {
	getAllCmd := s.client.HGetAll(s.key)
	values := getAllCmd.Val()
	newValues := make(map[string]interface{}, 0)
	for k, v := range values {
		newValues[k] = v
	}
	return newValues
}

// Set named val into session
func (s *session) Set(name string, val interface{}) {
	s.supportedHandle(name, func() {
		s.client.HSet(s.key, name, val)
	})
}

// SetAll values into session
func (s *session) SetAll(data map[string]interface{}, flush bool) {
	if flush {
		s.Clear()
	}
	_, have := data[sessionIdName]
	if have {
		delete(data, sessionIdName)
	}
	s.client.HMSet(s.key, data)
}

// Del named val from session
func (s *session) Del(name string) {
	s.supportedHandle(name, func() {
		s.client.HDel(s.key, name)
	})
}

// Clear session's values
func (s *session) Clear() {
	all := s.GetAll()
	ks := make([]string, 0)
	for k := range all {
		if k != sessionIdName {
			ks = append(ks, k)
		}
	}
	s.client.HDel(s.key, ks...)
}

func (s *session) supportedHandle(name string, fn func()) {
	if name != sessionIdName {
		fn()
	}
}
