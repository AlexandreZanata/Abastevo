package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	account "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	moderation "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/adapters"
	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters"
	"os"
	"time"
)

// Quality inspection shares the existing restricted CLI trust boundary. Text
// must only be rendered in a private operator terminal, never centralized logs.
func runQuality(args []string, getenv func(string) string) error {
	if len(args) == 0 || (args[0] != "inspect" && args[0] != "group" && args[0] != "queue") {
		return errors.New("quality requires queue, inspect or group")
	}
	fs := flag.NewFlagSet("quality "+args[0], flag.ContinueOnError)
	operator := fs.String("operator", "", "restricted operator identity")
	target := fs.String("target", "", "revision-bound review target UUID")
	station := fs.String("station", "", "station UUID")
	fuel := fs.String("fuel", "", "canonical fuel")
	day := fs.String("day", "", "incident date YYYY-MM-DD")
	after := fs.String("after", "", "last report UUID on previous page")
	if e := fs.Parse(args[1:]); e != nil {
		return e
	}
	if _, e := resolveOperatorID(*operator, getenv); e != nil {
		return e
	}
	if (args[0] == "inspect" && *target == "") || (args[0] == "group" && (*station == "" || *fuel == "" || *day == "")) {
		return errors.New("missing quality inspection target")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, _, e := openPool(ctx)
	if e != nil {
		return e
	}
	defer pool.Close()
	store := &quality.ReportStore{Pool: pool.Underlying()}
	var result any
	if args[0] == "queue" {
		items, next, err := moderation.NewStore(pool.Underlying()).QualityQueue(ctx, *after)
		e = err
		result = struct {
			Items any    `json:"items"`
			Next  string `json:"next_after_id,omitempty"`
		}{items, next}
	} else if args[0] == "inspect" {
		result, e = store.InspectReview(ctx, *target)
	} else {
		result, e = store.GroupReview(ctx, *station, *fuel, *day, *after, account.ActiveQualityOwnersTx)
	}
	if e != nil {
		return e
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}
