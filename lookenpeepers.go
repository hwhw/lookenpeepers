package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/util/exzerolog"
  "github.com/influxdata/influxdb-client-go/v2"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

var homeserver = flag.String("homeserver", "", "Matrix homeserver")
var userid = flag.String("userid", "", "Matrix user id")
var deviceid = flag.String("deviceid", "", "Matrix device id")
var accesstoken = flag.String("accesstoken", "", "Matrix access token")
var influxdb = flag.String("influxdb", "http://localhost:8086", "InfluxDB URL")
var influxdb_token = flag.String("influxdb_token", "", "InfluxDB access token")
var influxdb_org = flag.String("influxdb_org", "", "InfluxDB Org")
var influxdb_bucket = flag.String("influxdb_bucket", "lookenpeepers", "InfluxDB Bucket")
var debug = flag.Bool("debug", false, "Enable debug logs")

func main() {
	flag.Parse()
	if *userid == "" || *accesstoken == "" || *homeserver == "" || *deviceid == "" {
		_, _ = fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
		os.Exit(1)
	}

	client, err := mautrix.NewClient(*homeserver, "", "")
	if err != nil {
		panic(err)
	}

	log := zerolog.New(zerolog.NewConsoleWriter(func(w *zerolog.ConsoleWriter) {
		w.Out = rl.Stdout()
		w.TimeFormat = time.Stamp
	})).With().Timestamp().Logger()
	if !*debug {
		log = log.Level(zerolog.InfoLevel)
	}
	exzerolog.SetupDefaults(&log)
	client.Log = log

  influx_client := influxdb2.NewClient(*influxdb, *influxdb_token)
  influx_write := influx_client.WriteAPIBlocking(*influxdb_org, *influxdb_bucket)

  filter := mautrix.DefaultFilter()
  filter.Presence.NotSenders = []id.UserID{id.UserID(*userid)}
    
	syncer := client.Syncer.(*mautrix.DefaultSyncer)
  syncer.FilterJSON = &filter
	syncer.OnEventType(event.EphemeralEventPresence, func(ctx context.Context, evt *event.Event) {
		log.Debug().
			Str("sender", evt.Sender.String()).
			Str("Presence", string(evt.Content.AsPresence().Presence)).
			Int64("LastActiveAgo", evt.Content.AsPresence().LastActiveAgo).
			Bool("CurrentlyActive", evt.Content.AsPresence().CurrentlyActive).
			Str("StatusMessage", evt.Content.AsPresence().StatusMessage).
			Msg("Received presence event")

    if evt.Content.AsPresence().CurrentlyActive {
      p := influxdb2.NewPointWithMeasurement("presence").
        AddTag("user", evt.Sender.String()).
        AddField("online", true).
        SetTime(time.Now().Add(-time.Millisecond * time.Duration(evt.Content.AsPresence().LastActiveAgo)))
      err := influx_write.WritePoint(context.Background(), p)
      if err != nil {
        panic(err)
      }
    }
  })

	client.UserID = id.UserID(*userid)
	client.AccessToken = *accesstoken
  client.DeviceID = id.DeviceID(*deviceid)

	log.Info().Msg("Now running")
	syncCtx, _ := context.WithCancel(context.Background())
	var syncStopWait sync.WaitGroup
	syncStopWait.Add(1)

	go func() {
		err = client.SyncWithContext(syncCtx)
		defer syncStopWait.Done()
		if err != nil && !errors.Is(err, context.Canceled) {
			panic(err)
		}
	}()

	//cancelSync()
	syncStopWait.Wait()
  influx_client.Close()
}
