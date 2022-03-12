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
	"crypto/md5"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	r "github.com/go-redis/redis"

	s "github.com/go-the-way/anoweb/session"
)

const defaultPrefixKey = "session:"

type provider struct {
	mu        *sync.Mutex
	keyPrefix string
	options   *r.Options
	client    *r.Client
	sessions  map[string]s.Session
}

// Provider return new provider
func Provider(options *r.Options) *provider {
	return ProviderWithPrefixKey(options, defaultPrefixKey)
}

// ProviderWithPrefixKey return new provider with prefix key
func ProviderWithPrefixKey(options *r.Options, prefixKey string) *provider {
	client := r.NewClient(options)
	ping := client.Ping()
	p := &provider{&sync.Mutex{}, prefixKey, options, client, map[string]s.Session{}}
	if ping.Err() != nil {
		_, _ = fmt.Fprintln(os.Stderr, ping.Err())
	}
	p.syncSession()
	return p
}

// CookieName return cookie name
func (p *provider) CookieName() string {
	return "GOSESSID"
}

// GetId get session id
func (p *provider) GetId(r *http.Request) string {
	cookie, err := r.Cookie(p.CookieName())
	if err == nil && cookie != nil {
		return cookie.Value
	}
	return ""
}

func (p *provider) getRedisKey(id string) string {
	return fmt.Sprintf("%s%s", p.keyPrefix, id)
}

// Exists session
func (p *provider) Exists(id string) bool {
	currentSession := p.Get(id)
	return currentSession != nil && !currentSession.Invalidated()
}

// Get session
func (p *provider) Get(id string) s.Session {
	currentSession, have := p.sessions[id]
	if !have {
		return nil
	}
	return currentSession.(s.Session)
}

// Del session
func (p *provider) Del(id string) {
	p.del(id, true)
}

func (p *provider) del(id string, lock bool) {
	if lock {
		p.mu.Lock()
		defer p.mu.Unlock()
	}
	delCmd := p.client.Del(p.getRedisKey(id))
	if delCmd.Err() != nil {
		_, _ = fmt.Fprintln(os.Stderr, delCmd.Err())
	}
	delete(p.sessions, id)
}

func (p *provider) GetAll() map[string]s.Session {
	return p.sessions
}

// Clear session's values
func (p *provider) Clear() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for k := range p.sessions {
		p.del(k, false)
	}
}

func tmd5(text string) string {
	hashMd5 := md5.New()
	_, _ = io.WriteString(hashMd5, text)
	return fmt.Sprintf("%x", hashMd5.Sum(nil))
}

func newSID() string {
	nano := time.Now().UnixNano()
	rand.Seed(nano)
	rndNum := rand.Int63()
	return strings.ToUpper(tmd5(tmd5(strconv.FormatInt(nano, 10)) + tmd5(strconv.FormatInt(rndNum, 10))))
}

// New return new session
func (p *provider) New(config *s.Config, listener *s.Listener) s.Session {
	p.mu.Lock()
	defer p.mu.Unlock()
	sessionId := newSID()
	currentSession := newSession(p.client, sessionId, p.getRedisKey(sessionId))
	hashSetCmd := p.client.HSet(p.getRedisKey(sessionId), sessionIdName, sessionId)
	if hashSetCmd.Err() != nil {
		_, _ = fmt.Fprintln(os.Stderr, hashSetCmd.Err())
		return nil
	}
	expireCmd := p.client.Expire(p.getRedisKey(sessionId), config.Valid)
	if expireCmd.Err() != nil {
		_, _ = fmt.Fprintln(os.Stderr, expireCmd.Err())
		return nil
	}
	p.sessions[sessionId] = currentSession
	if listener != nil && listener.Created != nil {
		listener.Created(currentSession)
	}
	return currentSession
}

// Refresh session
func (p *provider) Refresh(session s.Session, config *s.Config, listener *s.Listener) {
	session.Renew(config.Valid)
	expireCmd := p.client.Expire(p.getRedisKey(session.Id()), config.Valid)
	if expireCmd.Err() != nil {
		_, _ = fmt.Fprintln(os.Stderr, expireCmd.Err())
	} else {
		go func() {
			if listener != nil && listener.Refreshed != nil {
				listener.Refreshed(session)
			}
		}()
	}
}

// Clean session
func (p *provider) Clean(_ *s.Config, listener *s.Listener) {
	go func() {
		for {
			p.cleanSession(listener)
			time.Sleep(time.Minute)
		}
	}()
}

func (p *provider) syncSession() {
	p.mu.Lock()
	defer p.mu.Unlock()
	var wg sync.WaitGroup
	wg.Add(1)
	go func(wg *sync.WaitGroup) {
		keysCmd := p.client.Keys(p.keyPrefix + "*")
		if keysCmd.Err() != nil {
			_, _ = fmt.Fprintln(os.Stderr, keysCmd.Err())
		} else {
			keys := keysCmd.Val()
			sessionMap := make(map[string]s.Session, 0)
			for _, key := range keys {
				hashGetAllCmd := p.client.HGetAll(key)
				if hashGetAllCmd.Err() != nil {
					_, _ = fmt.Fprintln(os.Stderr, hashGetAllCmd.Err())
					continue
				}
				values := hashGetAllCmd.Val()
				sessionId := values[sessionIdName]
				rs := newSession(p.client, sessionId, key)
				sessionMap[sessionId] = rs
				p.sessions[sessionId] = newSession(p.client, sessionId, key)
			}
		}
		wg.Done()
	}(&wg)
	wg.Wait()
}

func (p *provider) cleanSession(listener *s.Listener) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for sessionId, currentSession := range p.GetAll() {
		key := p.getRedisKey(sessionId)
		existsCmd := p.client.Exists(key)
		if existsCmd.Err() != nil {
			_, _ = fmt.Fprintln(os.Stderr, existsCmd.Err())
		} else {
			if existsCmd.Val() <= 0 {
				currentSession.Invalidate()
				go func() {
					if listener != nil && listener.Invalidated != nil {
						listener.Invalidated(currentSession)
					}
				}()
			}
		}
		if currentSession.Invalidated() {
			delete(p.sessions, sessionId)
			go func() {
				if listener != nil && listener.Destroyed != nil {
					listener.Destroyed(currentSession)
				}
			}()
		}
	}
}
