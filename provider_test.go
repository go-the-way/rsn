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
	"net/http"
	"os"
	"testing"
	"time"

	rds "github.com/go-redis/redis"

	s "github.com/go-the-way/anoweb/session"

	"github.com/stretchr/testify/require"
)

var (
	redisOptions = &rds.Options{
		Addr:     os.Getenv("TEST_REDIS_ADDR"),
		Password: os.Getenv("TEST_REDIS_PASSWORD"),
	}
)

func TestPing(t *testing.T) {
	c := rds.NewClient(redisOptions)
	defer func() {
		_ = c.Close()
	}()
	statusCmd := c.Ping()
	require.Nil(t, statusCmd.Err())
}

func TestProvider(t *testing.T) {
	p := Provider(redisOptions)
	require.NotNil(t, p.GetAll())
}

func TestProviderWithPrefixKey(t *testing.T) {
	p := ProviderWithPrefixKey(redisOptions, "_sessions_:")
	require.Equal(t, "_sessions_:", p.keyPrefix)
}

func TestProviderCookieName(t *testing.T) {
	p := Provider(redisOptions)
	require.Equal(t, "GOSESSID", p.CookieName())
}

func TestProviderGetId(t *testing.T) {
	p := Provider(redisOptions)
	req, _ := http.NewRequest("", "", nil)
	req.AddCookie(&http.Cookie{Name: p.CookieName(), Value: "hello---cookie---"})
	require.Equal(t, "hello---cookie---", p.GetId(req))
}

func TestProviderExists(t *testing.T) {
	p := Provider(redisOptions)
	currSession := p.New(&s.Config{Valid: time.Minute}, nil)
	require.NotNil(t, true, p.Exists(currSession.Id()))
	c := rds.NewClient(redisOptions)
	defer func() {
		_ = c.Close()
	}()
	hGetCmd := c.HGet("session:"+currSession.Id(), sessionIdName)
	if hGetCmd.Err() != nil {
		require.Error(t, hGetCmd.Err())
		return
	}
	require.Equal(t, currSession.Id(), hGetCmd.Val())
}

func TestProviderGet(t *testing.T) {
	p := Provider(redisOptions)
	c := rds.NewClient(redisOptions)
	defer func() {
		_ = c.Close()
	}()
	hSetCmd := c.HSet("session:xyz", sessionIdName, "xyz")
	if hSetCmd.Err() != nil {
		require.Error(t, hSetCmd.Err())
		return
	}
	expireCmd := c.Expire("session:xyz", time.Minute)
	if expireCmd.Err() != nil {
		require.Error(t, expireCmd.Err())
		return
	}
	p.syncSession()
	time.Sleep(time.Millisecond * 100)
	require.NotNil(t, p.Get("xyz"))
}

func TestProviderDel(t *testing.T) {
	p := Provider(redisOptions)
	currSession := p.New(&s.Config{Valid: time.Minute}, nil)
	p.Del(currSession.Id())
	require.NotNil(t, true, p.Exists(currSession.Id()))
	c := rds.NewClient(redisOptions)
	defer func() {
		_ = c.Close()
	}()
	keysCmd := c.Keys("session:" + currSession.Id())
	if keysCmd.Err() != nil {
		require.Error(t, keysCmd.Err())
		return
	}
	require.Zero(t, len(keysCmd.Val()))
}

func TestProviderGetAll(t *testing.T) {
	p := Provider(redisOptions)
	p.Clear()
	_ = p.New(&s.Config{Valid: time.Minute}, nil)
	c := rds.NewClient(redisOptions)
	defer func() {
		_ = c.Close()
	}()
	keysCmd := c.Keys("session:*")
	if keysCmd.Err() != nil {
		require.Error(t, keysCmd.Err())
		return
	}
	require.Equal(t, len(keysCmd.Val()), len(p.GetAll()))
	require.Equal(t, 1, len(keysCmd.Val()))
}

func TestProviderClear(t *testing.T) {
	p := Provider(redisOptions)
	p.Clear()
	c := rds.NewClient(redisOptions)
	defer func() {
		_ = c.Close()
	}()
	keysCmd := c.Keys("session:*")
	if keysCmd.Err() != nil {
		require.Error(t, keysCmd.Err())
		return
	}
	require.Equal(t, len(keysCmd.Val()), len(p.GetAll()))
	require.Equal(t, 0, len(p.GetAll()))
}
