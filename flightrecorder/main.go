package main

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/rivo/tview"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	eventspb "github.com/cilium/cilium/api/v1/events"
)

func main() {
	app := tview.NewApplication()
	agent := tview.NewTextView()
	agent.SetChangedFunc(func() {
		app.Draw()
	})
	agent.SetBorder(true)
	agent.SetText("waiting for status")
	go func() {
		if err := app.SetRoot(agent, true).Run(); err != nil {
			panic(err)
		}
	}()

	var opts []grpc.DialOption
	opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	conn, err := grpc.Dial("localhost:1234", opts...)
	if err != nil {
		log.Fatal("failed to dial: %w", err)
	}
	defer conn.Close()
	client := eventspb.NewFlightRecorderClient(conn)
	ctx := context.Background()
	stream, err := client.GetStatus(ctx, &eventspb.StatusUpdateRequest{})
	if err != nil {
		log.Fatal("failed to get status: %w", err)
	}
	modules := map[string]*eventspb.StatusUpdateEvent{}
	for {
		u, err := stream.Recv()
		if err != nil {
			log.Fatal("failed to receive: %w", err)
		}
		modules[u.ModuleId] = u
		buf := ""
		sortedKeys := []string{}
		for k, _ := range modules {
			sortedKeys = append(sortedKeys, k)
		}
		sort.Strings(sortedKeys)
		for _, k := range sortedKeys {
			status := modules[k]
			switch status.Type {
			case eventspb.Update_OK:
				// add green emoji
				buf += "✅ " + status.ModuleId + ":\t" + status.Type.String() + "\t" + time.Now().Format(time.RFC3339) + "\n"
			case eventspb.Update_DEGRADED:
				// add broken heart emoji
				buf += "💔 " + status.ModuleId + ":\t" + status.Type.String() + "\t" + time.Now().Format(time.RFC3339) + "\n"
				// todo: err
				if status.Error != nil {
					buf += strings.Repeat("-", 150) + "\n"
					buf += appendMessage(0, status.Error)
					buf += strings.Repeat("-", 150) + "\n"
				}
			case eventspb.Update_STOPPED:
				// add stop sign emoji
				buf += "🛑 " + status.ModuleId + ":\t" + status.Type.String() + "\t" + time.Now().Format(time.RFC3339) + "\n"
			}
		}
		agent.SetText(buf)
	}
}
func fmtMsg(s string, indent int) string {
	return strings.ReplaceAll(s, "\n", "\n"+strings.Repeat(".", indent))
}

func appendMessage(indent int, e *eventspb.Error) string {
	var buf string
	if e == nil {
		return buf
	}
	for _, h := range e.Help {
		if h == "" {
			continue
		}

		l := fmt.Sprintf("%s💬 Help: %s\n", strings.Repeat(" ", indent), h)
		buf += l
	}
	l := fmt.Sprintf("%s%s\n", strings.Repeat(" ", indent), fmtMsg(e.Message, indent))
	buf += l

	if len(e.Wrapped) >= 1 {
		for _, w := range e.Wrapped[:1] {
			if w != nil {
				buf += appendMessage(indent+2, w)
			}
		}
	}

	return buf
}
