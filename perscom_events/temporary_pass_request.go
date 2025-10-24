package perscom_events

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"
	"gorm.io/gorm"
)

const (
	temporaryPassRequestCustomID                    = "tpr"
	temporaryPassRequestSubmitCustomID              = "tpr-submit"
	temporaryPassRequestButtonApproveCustomIDPrefix = "tpr-approve"
	temporaryPassRequestButtonDenyCustomIDPrefix    = "tpr-deny"
	temporaryPassRequestDenyModalCustomID           = "tpr-deny-modal"
)

//go:embed temporary_pass_request_description.txt
var temporaryPassRequestDescription string

type TemporaryPassRequest struct {
	gorm.Model
	SubmittedByID snowflake.ID `gorm:"not null;index"` // Don't use, just for database compatibility
	SubmittedBy   *DiscordID   `gorm:"foreignKey:SubmittedByID;references:UserID" validate:"required"`
	SubmittedOn   time.Time    `validate:"required"`
	Operation     time.Time    `validate:"required"`
	IsApproved    bool
	IsProcessed   bool
	ProcessedByID snowflake.ID `gorm:"index"` // Don't use, just for database compatibility
	ProcessedBy   *DiscordID   `gorm:"foreignKey:ProcessedByID;references:UserID"`
}

type TemporaryPassRequests struct{}

func InitializeTemporaryPassRequestsFor(instance *GuildInstance) error {
	instance.Lock()
	defer instance.Unlock()

	err := instance.AutoMigrate(TemporaryPassRequest{})

	return err
}

func (tprs *TemporaryPassRequests) Len(ctxt context.Context, instance *GuildInstance) (int64, error) {
	instance.RLock()
	defer instance.RUnlock()

	l := int64(0)
	instance.WithContext(ctxt).Model(TemporaryPassRequest{}).Count(&l)
	return l, nil
}

func (tprs *TemporaryPassRequests) Add(ctxt context.Context, instance *GuildInstance, temporaryPassRequest TemporaryPassRequest) error {
	if err := validate.Struct(&temporaryPassRequest); err != nil {
		return errors.Join(ValidationError, err)
	}

	if temporaryPassRequest.IsApproved ||
		temporaryPassRequest.IsProcessed ||
		temporaryPassRequest.ProcessedBy != nil ||
		temporaryPassRequest.ProcessedByID != 0 {
		return fmt.Errorf("%w: a temporary pass request cannot be approved, reviewed, or denied before it has been submitted", ValidationError)
	}

	instance.Lock()
	defer instance.Unlock()

	count := int64(0)

	err := instance.WithContext(ctxt).Model(TemporaryPassRequest{}).
		Where("submitted_by_id = ?", temporaryPassRequest.SubmittedBy.UserID).
		Where("operation = ?", temporaryPassRequest.Operation).
		Count(&count).Error

	if err != nil {
		return errors.Join(DatabaseError, err)
	}

	if count > 0 {
		return fmt.Errorf("%w: a temporary pass request has already been submitted for %s", UserReportableError, temporaryPassRequest.SubmittedBy)
	}

	err = instance.WithContext(ctxt).Model(TemporaryPassRequest{}).Create(&temporaryPassRequest).Error

	if err != nil {
		err = errors.Join(DatabaseError, err)
	}

	return err
}

func (tprs *TemporaryPassRequests) Process(ctxt context.Context, instance *GuildInstance, userID string, processingUser *DiscordID, approved bool) error {
	if err := validate.Struct(processingUser); err != nil {
		return errors.Join(ValidationError, err)
	}

	if instance == nil {
		return fmt.Errorf("%w: guild instance can't be nil", ValidationError)
	}

	if err := validate.Var(userID, "required,numeric"); err != nil {
		return errors.Join(ValidationError, err)
	}

	instance.Lock()
	defer instance.Unlock()

	opTime := GetCurrentOpTime()

	storedTPRS := make([]TemporaryPassRequest, 0)
	err := instance.WithContext(ctxt).Model(TemporaryPassRequest{}).
		Where("submitted_by_id = ?", userID).
		Where("operation = ?", opTime).
		Find(&storedTPRS).Error

	if err != nil {
		return errors.Join(DatabaseError, err)
	}

	if len(storedTPRS) == 0 {
		return errors.Join(DatabaseError, fmt.Errorf("no temporary pass request found for user %s", userID))
	}

	if len(storedTPRS) > 1 {
		return errors.Join(DatabaseError, fmt.Errorf("multiple temporary pass requests found for user %s (this should be impossible)", userID))
	}

	storedTPR := storedTPRS[0]
	storedTPR.IsApproved = approved
	storedTPR.IsProcessed = true
	storedTPR.ProcessedBy = processingUser

	err = instance.WithContext(ctxt).Model(TemporaryPassRequest{}).
		Where("id = ?", storedTPR.ID).
		Save(storedTPR).Error
	if err != nil {
		err = errors.Join(DatabaseError, err)
	}

	return err
}

var temporaryPassRequests = new(TemporaryPassRequests)

var temporaryPassRequest = ButtonEventHandler{
	discord.NewPrimaryButton("Temporary Pass", temporaryPassRequestCustomID),
	[]bot.EventListener{
		temporaryPassRequestEventListener,
		temporaryPassRequestSubmitEventListener,
		temporaryPassRequestApprovalFlowHandler,
		temporaryPassRequestDeniedInputListener,
	},
}

var temporaryPassRequestEventListener = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
	var nickname string
	if event.Member() != nil && event.Member().Nick != nil {
		nickname = *event.Member().Nick
	} else {
		nickname = event.User().Username
	}
	userID := event.User().ID
	userName := event.User().Tag()

	discordIDForTriggerer := NewDiscordID(userID, userName, nickname)

	if event.Data.CustomID() == temporaryPassRequestCustomID {
		err := event.CreateMessage(
			discord.NewMessageCreateBuilder().
				SetEphemeral(true).
				SetEmbeds(discord.NewEmbedBuilder().
					SetTitle("Temporary Pass Request").
					SetColor(0x5765f2).
					SetDescription(temporaryPassRequestDescription).
					Build()).
				AddActionRow(discord.NewPrimaryButton("Submit", temporaryPassRequestSubmitCustomID)).
				Build(),
		)

		if err != nil {
			slog.Error("error while creating ephemeral TPR message for user",
				slog.Any("err", err),
				slog.String("triggered by", discordIDForTriggerer.UserID.String()),
				slog.String("submitted by", discordIDForTriggerer.UserID.String()),
			)
		}
	}
})

var temporaryPassRequestSubmitEventListener = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
	instance, ok := guildInstances[*event.GuildID()]
	if !ok {
		panic("no guild instance found for guild ID " + event.GuildID().String())
	}

	var nickname string
	if event.Member() != nil && event.Member().Nick != nil {
		nickname = *event.Member().Nick
	} else {
		nickname = event.User().Username
	}
	userID := event.User().ID
	userName := event.User().Tag()

	discordIDForTriggerer := NewDiscordID(userID, userName, nickname)

	if event.Data.CustomID() == temporaryPassRequestSubmitCustomID {
		temporaryPassRequest := TemporaryPassRequest{
			SubmittedBy: discordIDForTriggerer,
			SubmittedOn: time.Now().UTC(),
			Operation:   GetCurrentOpTime(),
		}

		ctxt, cncl := context.WithTimeout(context.Background(), defaultContextTimeout)
		err := temporaryPassRequests.Add(ctxt, instance, temporaryPassRequest)
		cncl()

		if err != nil {
			if errors.Is(err, UserReportableError) {
				// If it's a simple error that they caused, tell them and close the case
				_ = event.UpdateMessage(discord.NewMessageUpdateBuilder().
					ClearEmbeds().
					ClearContainerComponents().
					SetContent(fmt.Sprintf(":bangbang: %s. %s", err, helpSuffix)).
					Build(),
				)
			} else {
				// This is a bigger error that they probably didn't cause.
				slog.Error("error while adding temporary pass request",
					slog.Any("err", err),
					slog.String("triggered by", discordIDForTriggerer.UserID.String()),
					slog.String("submitted by", discordIDForTriggerer.UserID.String()),
				)

				_ = event.UpdateMessage(discord.NewMessageUpdateBuilder().
					ClearEmbeds().
					ClearContainerComponents().
					SetContent(":bangbang: I encountered an internal error. " + helpSuffix).
					Build(),
				)
			}
		} else {
			// Successfully submitted without error
			_, err = event.Client().Rest().CreateMessage(
				instance.SubmissionChannelID,
				discord.NewMessageCreateBuilder().
					SetContent(fmt.Sprintf(
						":pencil: __Temporary Pass Request__ (TPR) submitted by <@%s> for the operation on <t:%d:F>",
						userID, GetCurrentOpTime().Unix(),
					)).
					AddActionRow(
						discord.NewPrimaryButton("Approve", temporaryPassRequestButtonApproveCustomIDPrefix+userID.String()),
						discord.NewDangerButton("Deny", temporaryPassRequestButtonDenyCustomIDPrefix+userID.String()),
					).
					Build(),
			)

			if err != nil {
				slog.Error("error while creating approval message for s1", slog.Any("err", err))

				_ = event.UpdateMessage(discord.NewMessageUpdateBuilder().
					ClearEmbeds().
					ClearContainerComponents().
					SetContent(":bangbang: I encountered an internal error. " + helpSuffix).
					Build(),
				)
			} else {
				err = event.UpdateMessage(discord.NewMessageUpdateBuilder().
					ClearEmbeds().
					ClearContainerComponents().
					SetContent(":white_check_mark: Your temporary pass request has been submitted. You will receive updates via DM.").
					Build(),
				)

				if err != nil {
					slog.Error("failed to update TPR approval modal message for user!",
						slog.Any("err", err),
						slog.String("triggered by", discordIDForTriggerer.UserID.String()),
						slog.String("submitted by", discordIDForTriggerer.UserID.String()),
					)
				}
			}
		}
	}
})

var temporaryPassRequestApprovalFlowHandler = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
	instance, ok := guildInstances[*event.GuildID()]
	if !ok {
		panic("no guild instance found for guild ID " + event.GuildID().String())
	}

	var nickname string
	if event.Member() != nil && event.Member().Nick != nil {
		nickname = *event.Member().Nick
	} else {
		nickname = event.User().Username
	}

	userID := event.User().ID
	userName := event.User().Tag()

	discordIDForTriggerer := NewDiscordID(userID, userName, nickname)

	discordButtonCustomID := event.Data.CustomID()

	if strings.HasPrefix(discordButtonCustomID, temporaryPassRequestButtonApproveCustomIDPrefix) {
		temporaryPassRequestForUserID := strings.TrimPrefix(discordButtonCustomID, temporaryPassRequestButtonApproveCustomIDPrefix)

		ctxt, cncl := context.WithTimeout(context.Background(), defaultContextTimeout)
		err := temporaryPassRequests.Process(ctxt,
			instance,
			temporaryPassRequestForUserID,
			discordIDForTriggerer,
			true,
		)
		cncl()

		if err != nil {
			slog.Error("error while approving temporary pass request",
				slog.Any("err", err),
				slog.String("triggered by", discordIDForTriggerer.UserID.String()),
				slog.String("submitted by", temporaryPassRequestForUserID),
			)

			_ = event.UpdateMessage(discord.NewMessageUpdateBuilder().
				ClearEmbeds().
				ClearContainerComponents().
				SetContent(
					fmt.Sprintf("%s\n:bangbang: Error approving TPR (triggered by %s): %s.",
						event.Message.Content,
						discordIDForTriggerer,
						err),
				).Build(),
			)

			return
		} else {
			slog.Info("approved temporary pass request",
				slog.String("triggered by", discordIDForTriggerer.UserID.String()),
				slog.String("submitted by", temporaryPassRequestForUserID),
			)

			err = event.UpdateMessage(discord.NewMessageUpdateBuilder().
				ClearEmbeds().
				ClearContainerComponents().
				SetContent(
					fmt.Sprintf("%s\n:white_check_mark: **Approved** by %s",
						event.Message.Content,
						discordIDForTriggerer),
				).Build(),
			)

			if err != nil {
				slog.Error("failed to update TPR approval s1 message for user!",
					slog.Any("err", err),
					slog.String("triggered by", discordIDForTriggerer.UserID.String()),
					slog.String("submitted by", temporaryPassRequestForUserID),
				)
			}
		}

		dmChannel, err := event.Client().Rest().CreateDMChannel(snowflake.MustParse(temporaryPassRequestForUserID))
		if err == nil {
			_, _ = event.Client().Rest().CreateMessage(dmChannel.ID(),
				discord.NewMessageCreateBuilder().
					SetContent(":white_check_mark: Your temporary pass request has been **approved**.").
					Build(),
			)
		}

		// Let's not delete messages, just disable them and update info on them. This becomes a living log of progress
		//_ = event.Client().Rest().DeleteMessage(tprApprovalChannelID, event.Message.ID)

	} else if strings.HasPrefix(discordButtonCustomID, temporaryPassRequestButtonDenyCustomIDPrefix) {
		temporaryPassRequestForUserID := strings.TrimPrefix(discordButtonCustomID, temporaryPassRequestButtonDenyCustomIDPrefix)
		err := event.Modal(
			discord.NewModalCreateBuilder().
				SetTitle("Deny Temporary Pass").
				SetCustomID(temporaryPassRequestDenyModalCustomID + temporaryPassRequestForUserID).
				AddActionRow(discord.NewShortTextInput("deny-reason", "Reason for denial (required)")).
				Build(),
		)

		if err != nil {
			slog.Error("failed to show deny modal",
				slog.Any("err", err),
				slog.String("triggered by", discordIDForTriggerer.UserID.String()),
				slog.String("submitted by", temporaryPassRequestForUserID),
			)
		}
	}
})

var temporaryPassRequestDeniedInputListener = bot.NewListenerFunc(func(event *events.ModalSubmitInteractionCreate) {
	instance, ok := guildInstances[*event.GuildID()]
	if !ok {
		panic("no guild instance found for guild ID " + event.GuildID().String())
	}

	var nickname string
	if event.Member() != nil && event.Member().Nick != nil {
		nickname = *event.Member().Nick
	} else {
		nickname = event.User().Username
	}

	userID := event.User().ID
	userName := event.User().Tag()

	discordIDForTriggerer := NewDiscordID(userID, userName, nickname)

	discordButtonCustomID := event.Data.CustomID

	if strings.HasPrefix(discordButtonCustomID, temporaryPassRequestDenyModalCustomID) {
		temporaryPassRequestForUserID := strings.TrimPrefix(discordButtonCustomID, temporaryPassRequestDenyModalCustomID)

		reason, _ := event.Data.TextInputComponent("deny-reason")
		ctxt, cncl := context.WithTimeout(context.Background(), defaultContextTimeout)
		err := temporaryPassRequests.Process(ctxt,
			instance,
			temporaryPassRequestForUserID,
			discordIDForTriggerer,
			false,
		)
		cncl()

		if err != nil {
			slog.Error("error while denying temporary pass request",
				slog.Any("err", err),
				slog.String("triggered by", discordIDForTriggerer.UserID.String()),
				slog.String("submitted by", temporaryPassRequestForUserID),
			)

			_ = event.UpdateMessage(discord.NewMessageUpdateBuilder().
				ClearEmbeds().
				ClearContainerComponents().
				SetContent(
					fmt.Sprintf("%s\n:bangbang: Error denying TPR (triggered by %s): %s.",
						event.Message.Content,
						discordIDForTriggerer,
						err),
				).Build(),
			)

			return
		} else {
			slog.Info("denied temporary pass request",
				slog.String("triggered by", discordIDForTriggerer.UserID.String()),
				slog.String("submitted by", temporaryPassRequestForUserID),
			)

			err = event.UpdateMessage(discord.NewMessageUpdateBuilder().
				ClearEmbeds().
				ClearContainerComponents().
				SetContent(
					fmt.Sprintf("%s\n:x: **Denied** by %s for \"%s\".",
						event.Message.Content,
						discordIDForTriggerer,
						reason.Value,
					),
				).Build(),
			)

			if err != nil {
				slog.Error("failed to update TPR approval s1 message for user!",
					slog.Any("err", err),
					slog.String("triggered by", discordIDForTriggerer.UserID.String()),
					slog.String("submitted by", temporaryPassRequestForUserID),
				)
			}
		}

		dmChannel, err := event.Client().Rest().CreateDMChannel(snowflake.MustParse(temporaryPassRequestForUserID))
		if err == nil {
			_, _ = event.Client().Rest().CreateMessage(dmChannel.ID(),
				discord.NewMessageCreateBuilder().
					SetContentf(":x: Your temporary pass request has been **denied** for \"%s\". %s",
						reason.Value,
						helpSuffix,
					).
					Build(),
			)
		}
	}
})

//var temporaryPassRequestDeniedEventListener = bot.NewListenerFunc(func(event *events.ApplicationCommandInteractionCreate) {
//	if event.Data.CommandName() == "tpr-list" {
//		tprRequestsMutex.Lock()
//		defer tprRequestsMutex.Unlock()
//		if len(temporaryPassRequests) == 0 {
//			event.CreateMessage(discord.NewMessageCreateBuilder().
//				SetContent("No approved temporary pass requests.").
//				Build())
//			return
//		}
//		var content strings.Builder
//		content.WriteString("**Approved Temporary Pass Requests:**\n")
//		for _, tpr := range temporaryPassRequests {
//			if tpr.Approved != nil && *tpr.Approved {
//				content.WriteString(fmt.Sprintf("- <@%s> for <t:%d:R>\n", tpr.UserID, tpr.Operation.Unix()))
//			}
//		}
//		event.CreateMessage(discord.NewMessageCreateBuilder().
//			SetContent(content.String()).
//			Build())
//	}
//})
//
//func updateTPRForumPost(client bot.Client) {
//	tprRequestsMutex.Lock()
//	defer tprRequestsMutex.Unlock()
//
//	var newEntries strings.Builder
//
//	for _, req := range temporaryPassRequests {
//		if req.Approved == nil {
//			continue
//		}
//		key := fmt.Sprintf("%s|%s|%v", req.UserID, req.Operation.Format("2006-01-02"), req.Approved)
//		tprForumAddedMutex.Lock()
//		if tprForumAdded[key] {
//			tprForumAddedMutex.Unlock()
//			continue
//		}
//		tprForumAdded[key] = true
//		tprForumAddedMutex.Unlock()
//		status := "🕐 pending"
//		if *req.Approved {
//			status = "✅ approved by " + req.ProcessedBy
//		} else {
//			status = fmt.Sprintf("❌ denied by %s — Reason: **%s**", req.ProcessedBy, req.DeniedReason)
//		}
//		newEntries.WriteString(fmt.Sprintf(
//			"\n• %s submitted TPR at %s for: <t:%d:F> (Status: %s)\n",
//			req.Nickname,
//			req.SubmittedOn.In(time.FixedZone("CST", -6*60*60)).Format("Mon, 02 Jan 2006 15:04 MST"),
//			req.Operation.Unix(),
//			status,
//		))
//	}
//
//	if newEntries.Len() == 0 {
//		return
//	}
//
//	if tprForumMessageID == 0 {
//		msg, err := client.Rest().CreateMessage(tprForumThreadID,
//			discord.NewMessageCreateBuilder().
//				SetContent("**Temporary Pass Request Log:**\n"+newEntries.String()).
//				Build())
//		if err != nil {
//			slog.Error("failed to create TPR forum post", slog.Any("err", err))
//			return
//		}
//		tprForumMessageID = msg.ID
//	} else {
//		msg, err := client.Rest().GetMessage(tprForumThreadID, tprForumMessageID)
//		if err != nil {
//			slog.Error("failed to fetch TPR forum post", slog.Any("err", err))
//			return
//		}
//		newContent := msg.Content + newEntries.String()
//		if len(newContent) > 2000 {
//			newContent = newContent[:2000]
//		}
//		_, err = client.Rest().UpdateMessage(tprForumThreadID, tprForumMessageID,
//			discord.NewMessageUpdateBuilder().SetContent(newContent).Build())
//		if err != nil {
//			slog.Error("failed to update TPR forum post", slog.Any("err", err))
//		}
//	}
//}
//
//func InitTPRScheduler(client bot.Client) {
//	go func() {
//		for {
//			now := time.Now().UTC()
//			daysUntilSunday := (7 - int(now.Weekday())) % 7
//			nextSunday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, daysUntilSunday)
//			duration := nextSunday.Sub(now)
//			if duration <= 0 {
//				// If duration is zero or negative, sleep for 1 second to avoid tight loop
//				time.Sleep(time.Second)
//				continue
//			}
//			time.Sleep(duration)
//
//			tprRequestsMutex.Lock()
//			temporaryPassRequests = nil
//			tprRequestsMutex.Unlock()
//			tprForumMessageID = 0
//			slog.Info("Cleared TPRs and forum post for new week")
//		}
//	}()
//}
