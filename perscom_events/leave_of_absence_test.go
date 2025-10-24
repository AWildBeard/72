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

func TestLeaveOfAbsenseRequests_Add(t *testing.T) {
	testCases := []struct {
		name               string
		parallel           bool
		want               error
		setupAdds          []LeaveOfAbsenseRequest
		setupAddAndProcess []struct {
			approve bool
			loa     LeaveOfAbsenseRequest
		}
		submissions []LeaveOfAbsenseRequest
	}{
		{
			name:     "Correct submission does not error",
			parallel: true,
			want:     nil,
			submissions: []LeaveOfAbsenseRequest{
				{
					SubmittedBy:   NewDiscordID(snowflake.MustParse("1"), "is", "a"),
					SubmittedOn:   time.Now(),
					Reason:        "Short break",
					ReturnETA:     "Next month",
					HasReturned:   false,
					IsApproved:    false,
					IsProcessed:   false,
					ProcessedByID: 0,
					ProcessedBy:   nil,
				},
			},
		},
		{
			name:     "Duplicate submission generates error",
			parallel: true,
			want:     UserReportableError,
			setupAdds: []LeaveOfAbsenseRequest{
				{
					SubmittedBy:   NewDiscordID(snowflake.MustParse("1"), "is", "a"),
					SubmittedOn:   time.Now(),
					Reason:        "Short break",
					ReturnETA:     "Next month",
					HasReturned:   false,
					IsApproved:    false,
					IsProcessed:   false,
					ProcessedByID: 0,
					ProcessedBy:   nil,
				},
			},
			submissions: []LeaveOfAbsenseRequest{
				{
					SubmittedBy:   NewDiscordID(snowflake.MustParse("1"), "is", "a"),
					SubmittedOn:   time.Now(),
					Reason:        "Short break",
					ReturnETA:     "Next month",
					HasReturned:   false,
					IsApproved:    false,
					IsProcessed:   false,
					ProcessedByID: 0,
					ProcessedBy:   nil,
				},
			},
		},
		{
			name:     "Another submission after approval generates error",
			parallel: true,
			want:     UserReportableError,
			setupAddAndProcess: []struct {
				approve bool
				loa     LeaveOfAbsenseRequest
			}{
				{
					true,
					LeaveOfAbsenseRequest{
						SubmittedBy:   NewDiscordID(snowflake.MustParse("1"), "is", "a"),
						SubmittedOn:   time.Now(),
						Reason:        "Short break",
						ReturnETA:     "Next month",
						HasReturned:   false,
						IsApproved:    false,
						IsProcessed:   false,
						ProcessedByID: 0,
						ProcessedBy:   nil,
					},
				},
			},
			submissions: []LeaveOfAbsenseRequest{
				{
					SubmittedBy:   NewDiscordID(snowflake.MustParse("1"), "is", "a"),
					SubmittedOn:   time.Now(),
					Reason:        "Short break",
					ReturnETA:     "Next month",
					HasReturned:   false,
					IsApproved:    false,
					IsProcessed:   false,
					ProcessedByID: 0,
					ProcessedBy:   nil,
				},
			},
		},
		{
			name:     "Another submission after denial is accepted",
			parallel: true,
			want:     nil,
			setupAddAndProcess: []struct {
				approve bool
				loa     LeaveOfAbsenseRequest
			}{
				{
					false,
					LeaveOfAbsenseRequest{
						SubmittedBy:   NewDiscordID(snowflake.MustParse("1"), "is", "a"),
						SubmittedOn:   time.Now(),
						Reason:        "Short break",
						ReturnETA:     "Next month",
						HasReturned:   false,
						IsApproved:    false,
						IsProcessed:   false,
						ProcessedByID: 0,
						ProcessedBy:   nil,
					},
				},
			},
			submissions: []LeaveOfAbsenseRequest{
				{
					SubmittedBy:   NewDiscordID(snowflake.MustParse("1"), "is", "a"),
					SubmittedOn:   time.Now(),
					Reason:        "Short break",
					ReturnETA:     "Next month",
					HasReturned:   false,
					IsApproved:    false,
					IsProcessed:   false,
					ProcessedByID: 0,
					ProcessedBy:   nil,
				},
			},
		},
		{
			name:     "Approved submission generates error",
			parallel: true,
			want:     ValidationError,
			submissions: []LeaveOfAbsenseRequest{
				{
					SubmittedBy:   NewDiscordID(snowflake.MustParse("1"), "is", "a"),
					SubmittedOn:   time.Now(),
					Reason:        "Short break",
					ReturnETA:     "Next month",
					HasReturned:   false,
					IsApproved:    true,
					IsProcessed:   false,
					ProcessedByID: 0,
					ProcessedBy:   nil,
				},
			},
		},
		{
			name:     "Processed submission generates error",
			parallel: true,
			want:     ValidationError,
			submissions: []LeaveOfAbsenseRequest{
				{
					SubmittedBy:   NewDiscordID(snowflake.MustParse("1"), "is", "a"),
					SubmittedOn:   time.Now(),
					Reason:        "Short break",
					ReturnETA:     "Next month",
					HasReturned:   false,
					IsApproved:    false,
					IsProcessed:   true,
					ProcessedByID: 0,
					ProcessedBy:   nil,
				},
			},
		},
		{
			name:     "Marked returned submission generates error",
			parallel: true,
			want:     ValidationError,
			submissions: []LeaveOfAbsenseRequest{
				{
					SubmittedBy:   NewDiscordID(snowflake.MustParse("1"), "is", "a"),
					SubmittedOn:   time.Now(),
					Reason:        "Short break",
					ReturnETA:     "Next month",
					HasReturned:   true,
					IsApproved:    false,
					IsProcessed:   false,
					ProcessedByID: 0,
					ProcessedBy:   nil,
				},
			},
		},
		{
			name:     "marked processed submission generates error",
			parallel: true,
			want:     ValidationError,
			submissions: []LeaveOfAbsenseRequest{
				{
					SubmittedBy:   NewDiscordID(snowflake.MustParse("1"), "is", "a"),
					SubmittedOn:   time.Now(),
					Reason:        "Short break",
					ReturnETA:     "Next month",
					HasReturned:   false,
					IsApproved:    false,
					IsProcessed:   false,
					ProcessedByID: 0,
					ProcessedBy:   NewDiscordID(snowflake.MustParse("8"), "is", "a"),
				},
			},
		},
		{
			name:     "marked processed submission generates error 2",
			parallel: true,
			want:     ValidationError,
			submissions: []LeaveOfAbsenseRequest{
				{
					SubmittedBy:   NewDiscordID(snowflake.MustParse("1"), "is", "a"),
					SubmittedOn:   time.Now(),
					Reason:        "Short break",
					ReturnETA:     "Next month",
					HasReturned:   false,
					IsApproved:    false,
					IsProcessed:   false,
					ProcessedByID: 9,
					ProcessedBy:   nil,
				},
			},
		},
		{
			name:     "marked returned by submission generates error",
			parallel: true,
			want:     ValidationError,
			submissions: []LeaveOfAbsenseRequest{
				{
					SubmittedBy:        NewDiscordID(snowflake.MustParse("1"), "is", "a"),
					SubmittedOn:        time.Now(),
					Reason:             "Short break",
					ReturnETA:          "Next month",
					HasReturned:        false,
					IsApproved:         false,
					IsProcessed:        false,
					ProcessedByID:      0,
					ProcessedBy:        nil,
					MarkedReturnedByID: 9,
					MarkedReturnedBy:   nil,
				},
			},
		},
		{
			name:     "marked returned by submission generates error 2",
			parallel: true,
			want:     ValidationError,
			submissions: []LeaveOfAbsenseRequest{
				{
					SubmittedBy:        NewDiscordID(snowflake.MustParse("1"), "is", "a"),
					SubmittedOn:        time.Now(),
					Reason:             "Short break",
					ReturnETA:          "Next month",
					HasReturned:        false,
					IsApproved:         false,
					IsProcessed:        false,
					ProcessedByID:      0,
					ProcessedBy:        nil,
					MarkedReturnedByID: 0,
					MarkedReturnedBy:   NewDiscordID(snowflake.MustParse("8"), "is", "a"),
				},
			},
		},
	}

	for index, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.parallel {
				t.Parallel()
			}

			loas := new(LeaveOfAbsenseRequests)

			db, err := gorm.Open(sqlite.Open(
				fmt.Sprintf("%s/test-leave-of-absence-requests-add-%d.db",
					os.TempDir(),
					index,
				)),
				&gorm.Config{})

			if err != nil {
				t.Fatal(err)
			}

			defer func() {
				d, _ := db.DB()
				if d != nil {
					d.Close()
				}
			}()
			defer os.Remove(fmt.Sprintf("%s/test-leave-of-absence-requests-add-%d.db",
				os.TempDir(),
				index))

			err = db.AutoMigrate(LeaveOfAbsenseRequest{})

			if err != nil {
				t.Fatal(err)
			}

			instance := &GuildInstance{
				DB:      db,
				RWMutex: sync.RWMutex{},
			}

			errs := make([]error, 0)

			for _, setupAdd := range tc.setupAdds {
				err = loas.Add(context.Background(), instance, setupAdd)
				if err != nil {
					t.Fatal(err)
				}
			}

			for _, setupAndProcess := range tc.setupAddAndProcess {
				err = loas.Add(context.Background(), instance, setupAndProcess.loa)
				if err != nil {
					t.Fatal(err)
				}

				err = loas.Process(context.Background(), instance, setupAndProcess.loa.SubmittedBy.UserID.String(), NewDiscordID(snowflake.MustParse("8"), "is", "a"), setupAndProcess.approve)
				if err != nil {
					t.Fatal(err)
				}
			}

			for _, submission := range tc.submissions {
				err = loas.Add(context.Background(), instance, submission)

				if err != nil {
					errs = append(errs, err)
				}
			}

			err = errors.Join(errs...)

			assert.ErrorIsf(t, err, tc.want, "got %v, want %v", err, tc.want)
		})
	}
}
