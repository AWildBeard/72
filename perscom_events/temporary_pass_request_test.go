package perscom_events

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

func TestTemporaryPassRequests_Add(t *testing.T) {
	testCases := []struct {
		name             string
		parallel         bool
		want             error
		setupSubmissions []TemporaryPassRequest
		submissions      []TemporaryPassRequest
	}{
		{
			name:     "nil instance is rejected",
			parallel: true,
			submissions: []TemporaryPassRequest{
				{
					SubmittedByID: 0,
					SubmittedBy:   nil,
					SubmittedOn:   time.Time{},
					Operation:     time.Time{},
					IsApproved:    false,
					IsProcessed:   false,
					ProcessedByID: 0,
					ProcessedBy:   nil,
				},
			},
			want: ValidationError,
		},
		{
			name:     "Empty TPR is rejected with a DatabaseError",
			parallel: true,
			submissions: []TemporaryPassRequest{
				{
					SubmittedByID: 0,
					SubmittedBy:   nil,
					SubmittedOn:   time.Time{},
					Operation:     time.Time{},
					IsApproved:    false,
					IsProcessed:   false,
					ProcessedByID: 0,
					ProcessedBy:   nil,
				},
			},
			want: ValidationError,
		},
		{
			name:     "Initial TPR waiting for approval",
			parallel: true,
			submissions: []TemporaryPassRequest{
				{
					SubmittedByID: 1,
					SubmittedBy:   NewDiscordID(snowflake.MustParse("1"), "is", "a"),
					SubmittedOn:   time.Now(),
					Operation:     GetCurrentOpTime(),
					IsApproved:    false,
					IsProcessed:   false,
					ProcessedByID: 0,
					ProcessedBy:   nil,
				},
			},
			want: nil,
		},
		{
			name:     "Duplicate TPR generates error",
			parallel: true,
			setupSubmissions: []TemporaryPassRequest{
				{
					SubmittedByID: 1,
					SubmittedBy:   NewDiscordID(snowflake.MustParse("1"), "is", "a"),
					SubmittedOn:   time.Now(),
					Operation:     GetCurrentOpTime(),
					IsApproved:    false,
					IsProcessed:   false,
					ProcessedByID: 0,
					ProcessedBy:   nil,
				},
			},
			submissions: []TemporaryPassRequest{
				{
					SubmittedByID: 1,
					SubmittedBy:   NewDiscordID(snowflake.MustParse("1"), "is", "a"),
					SubmittedOn:   time.Now(),
					Operation:     GetCurrentOpTime(),
					IsApproved:    false,
					IsProcessed:   false,
					ProcessedByID: 0,
					ProcessedBy:   nil,
				},
			},
			want: UserReportableError,
		},
		{
			name:     "A TPR can't be submitted as approved",
			parallel: true,
			submissions: []TemporaryPassRequest{
				{
					SubmittedByID: 1,
					SubmittedBy:   NewDiscordID(snowflake.MustParse("1"), "is", "a"),
					SubmittedOn:   time.Now(),
					Operation:     GetCurrentOpTime(),
					IsApproved:    true,
					IsProcessed:   false,
					ProcessedByID: 0,
					ProcessedBy:   nil,
				},
			},
			want: ValidationError,
		},
		{
			name:     "A TPR can't be submitted as reviewed",
			parallel: true,
			submissions: []TemporaryPassRequest{
				{
					SubmittedByID: 1,
					SubmittedBy:   NewDiscordID(snowflake.MustParse("1"), "is", "a"),
					SubmittedOn:   time.Now(),
					Operation:     GetCurrentOpTime(),
					IsApproved:    false,
					IsProcessed:   true,
					ProcessedByID: 0,
					ProcessedBy:   nil,
				},
			},
			want: ValidationError,
		},
		{
			name:     "A TPR can't be submitted with reviewer",
			parallel: true,
			submissions: []TemporaryPassRequest{
				{
					SubmittedByID: 1,
					SubmittedBy:   NewDiscordID(snowflake.MustParse("1"), "is", "a"),
					SubmittedOn:   time.Now(),
					Operation:     GetCurrentOpTime(),
					IsApproved:    false,
					IsProcessed:   false,
					ProcessedByID: 1,
					ProcessedBy:   nil,
				},
			},
			want: ValidationError,
		},
		{
			name:     "A TPR can't be submitted with reviewer struct",
			parallel: true,
			submissions: []TemporaryPassRequest{
				{
					SubmittedByID: 1,
					SubmittedBy:   NewDiscordID(snowflake.MustParse("1"), "is", "a"),
					SubmittedOn:   time.Now(),
					Operation:     GetCurrentOpTime(),
					IsApproved:    false,
					IsProcessed:   false,
					ProcessedByID: 0,
					ProcessedBy:   NewDiscordID(snowflake.MustParse("1"), "is", "a"),
				},
			},
			want: ValidationError,
		},
	}

	for index, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.parallel {
				t.Parallel()
			}

			db, err := gorm.Open(sqlite.Open(
				fmt.Sprintf("%s/test-temporary-pass-requests-add-%d.db",
					os.TempDir(),
					index,
				)), &gorm.Config{},
			)
			if err != nil {
				t.Fatal(err)
			}

			defer func() {
				d, _ := db.DB()
				if d != nil {
					d.Close()
				}
			}()

			defer os.Remove(fmt.Sprintf("%s/test-temporary-pass-requests-add-%d.db",
				os.TempDir(),
				index,
			))

			db.AutoMigrate(&TemporaryPassRequest{})

			tprs := TemporaryPassRequests{}

			instance := &GuildInstance{
				DB:      db,
				RWMutex: sync.RWMutex{},
			}

			errs := make([]error, 0)

			for _, setup := range tc.setupSubmissions {
				err = tprs.Add(context.Background(), instance, setup)
				if err != nil {
					t.Fatal(err)
				}
			}

			for _, submission := range tc.submissions {
				err := tprs.Add(context.Background(), instance, submission)

				if err != nil {
					errs = append(errs, err)
				}
			}

			err = errors.Join(errs...)

			assert.ErrorIsf(t, err, tc.want, "got %v, want %v", err, tc.want)
		})
	}
}
