package main

import (
	"encoding/json"
	"time"

	// "fmt"
	"log"
	"regexp"
	"sync"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)


type SubscriptionBody struct {
    ClientId string `json:"clientId"`
    Subscriptions []string `json:"subscriptions"`
}

type StringCache struct {
    data map[string]string
    mutex sync.RWMutex
}

type Computer struct {
    Id string `db:"id" json:"id"`
    Token string `db:"token" json:"token"`
    Status string `db:"status" json:"status"`
}

func NewStringCache() *StringCache {
    return &StringCache{
        data: make(map[string]string),
    }
}

func (c *StringCache) Set(key, value string) {
    c.mutex.Lock()
    defer c.mutex.Unlock()
    c.data[key] = value
}

func (c *StringCache) Get(key string) (string, bool) {
    c.mutex.RLock()
    defer c.mutex.RUnlock()
    value, exists := c.data[key]
    return value, exists
}

func (c *StringCache) Delete(key string) {
    c.mutex.Lock()
    defer c.mutex.Unlock()
    delete(c.data, key)
}

var globalCache *StringCache

func main() {
    app := pocketbase.New()
    globalCache = NewStringCache()
    token_re := regexp.MustCompile("\\?options=.*token%22%3A%20%22([^%]*)%22")

    app.OnRealtimeAfterSubscribeRequest().Add(func(e *core.RealtimeSubscribeEvent) error {
        data := SubscriptionBody{}
        decoder := json.NewDecoder(e.HttpContext.Request().Body)

        if err := decoder.Decode(&data); err != nil {
            return err
        }

        if len(data.Subscriptions) < 1 {
            return nil
        }

        matches := token_re.FindStringSubmatch(data.Subscriptions[0])

        if len(matches) < 2 {
            return nil
        }

        token := matches[1]
        
        if token == "" {
            return nil
        }

        globalCache.Set(data.ClientId, token)
        return nil
    })

    app.OnRealtimeConnectRequest().Add(func(e *core.RealtimeConnectEvent) error {
        e.IdleTimeout = time.Minute * 13 // A Hour
        return nil
    })


    app.OnRealtimeDisconnectRequest().Add(func(e *core.RealtimeDisconnectEvent) error {
        token, ok := globalCache.Get(e.Client.Id())
        globalCache.Delete(e.Client.Id())
        
        if !ok {
            return nil
        }

        // fmt.Println(token)

        computer := Computer{}
        err := app.Dao().DB().NewQuery("SELECT id, token, status FROM computers WHERE token = {:token}").Bind(dbx.Params{"token": token}).One(&computer)

        if err != nil {
            return err
        }

        if computer.Status != "0" {
            record, err := app.Dao().FindFirstRecordByFilter("computers", "token = {:token}", dbx.Params{"token": token})
            if err != nil {
                return err
            }

            record.Set("status", 0)

            if err := app.Dao().SaveRecord(record); err != nil {
                return err
            }
        }

        // fmt.Println(computer)

        return nil
    })

    if err := app.Start(); err != nil {
        log.Fatal(err)
    }
}