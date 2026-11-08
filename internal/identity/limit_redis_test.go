package identity

import (
 "context"
 "os"
 "testing"
 "time"

 "github.com/google/uuid"
)

func TestSharedLimitsAcrossClients(t *testing.T) {
 address := os.Getenv("SHIPPIN_TEST_REDIS_URL")
 if address == "" { t.Skip("SHIPPIN_TEST_REDIS_URL not set") }
 prefix := "shippin:test:"+uuid.NewString()+":"
 first, err := NewSharedLimits(context.Background(),address,prefix)
 if err != nil { t.Fatal(err) }; defer first.Close()
 second, err := NewSharedLimits(context.Background(),address,prefix)
 if err != nil { t.Fatal(err) }; defer second.Close()
 a := &sharedLimiter{first,"test",2,250*time.Millisecond}
 b := &sharedLimiter{second,"test",2,250*time.Millisecond}
 if !a.Allow("client",time.Now()) || !b.Allow("client",time.Now()) || a.Allow("client",time.Now()) { t.Fatal("clients did not enforce one shared window") }
 time.Sleep(300*time.Millisecond)
 if !b.Allow("client",time.Now()) { t.Fatal("expired counter still denied") }
 first.Close()
 if a.Allow("another-client",time.Now()) { t.Fatal("unavailable store allowed an attempt") }
}
