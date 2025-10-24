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

const leaveOfAbsenceCustomID = "loa"
const leaveOfAbsenceModalCustomID = "loa-modal"
const leaveOfAbsenceModalSubmissionCustomID = "loa-modal-submit"
const leaveOfAbsenceApproveButtonCustomID = "loa-approve"
const leaveOfAbsenceDenyButtonCustomID = "loa-deny"
const leaveOfAbsenceDenyModalCustomID = "loa-deny-modal"

//go:embed leave_of_absence_description.txt
var leaveOfAbsenceDescription string

type LeaveOfAbsenseRequest struct {
	gorm.Model
	SubmittedByID      snowflake.ID `gorm:"not null;index"` // Don't use, just for database compatibility
	SubmittedBy        *DiscordID   `gorm:"foreignKey:SubmittedByID;references:UserID" validate:"required"`
	SubmittedOn        time.Time    `validate:"required"`
	Reason             string       `validate:"required"`
	ReturnETA          string       `validate:"required"`
	IsApproved         bool
	HasReturned        bool
	MarkedReturnedByID snowflake.ID `gorm:"index"`
	MarkedReturnedBy   *DiscordID   `gorm:"foreignKey:MarkedReturnedByID;references:UserID"`
	IsProcessed        bool
	ProcessedByID      snowflake.ID `gorm:"index"` // Don't use, just for database compatibility
	ProcessedBy        *DiscordID   `gorm:"foreignKey:ProcessedByID;references:UserID"`
}

type LeaveOfAbsenseRequests struct{}

func InitializeLeaveOfAbsenceRequestsFor(instance *GuildInstance) error {
	instance.Lock()
	defer instance.Unlock()

	err := instance.AutoMigrate(LeaveOfAbsenseRequest{})

	return err
}

func (l *LeaveOfAbsenseRequests) Add(ctxt context.Context, instance *GuildInstance, loa LeaveOfAbsenseRequest) error {
	if err := validate.Struct(loa); err != nil {
		return errors.Join(ValidationError, err)
	}

	if loa.IsApproved ||
		loa.IsProcessed ||
		loa.HasReturned ||
		loa.ProcessedBy != nil ||
		loa.ProcessedByID != 0 ||
		loa.MarkedReturnedByID != 0 ||
		loa.MarkedReturnedBy != nil {
		return fmt.Errorf("%w: a leave of absence request cannot be approved, reviewed, denied, or marked returned before it has been submitted", ValidationError)
	}

	instance.Lock()
	defer instance.Unlock()

	count := int64(0)

	err := instance.WithContext(ctxt).Model(LeaveOfAbsenseRequest{}).
		Where("submitted_by_id = ?", loa.SubmittedBy.UserID).
		Where("has_returned = ? and is_processed = ?", false, false).                       // Not returned and not processed
		Or("has_returned = ? and is_processed = ? and is_approved = ?", false, true, true). // not returned, processed, was approved
		Count(&count).Error

	if err != nil {
		return errors.Join(DatabaseError, err)
	}

	if count > 0 {
		return fmt.Errorf("%w: a leave of absence request has already been submitted for %s", UserReportableError, loa.SubmittedBy)
	}

	err = instance.WithContext(ctxt).Model(LeaveOfAbsenseRequest{}).Create(&loa).Error

	if err != nil {
		err = errors.Join(DatabaseError, err)
	}

	return err
}

func (l *LeaveOfAbsenseRequests) Process(ctxt context.Context, instance *GuildInstance, userID string, processingUser *DiscordID, approved bool) error {
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

	storedLOAS := make([]LeaveOfAbsenseRequest, 0)
	err := instance.WithContext(ctxt).Model(LeaveOfAbsenseRequest{}).
		Where("submitted_by_id = ?", userID).
		Where("has_returned = ?", false).
		Where("is_processed = ?", false).
		Find(&storedLOAS).Error

	if err != nil {
		return errors.Join(DatabaseError, err)
	}

	if len(storedLOAS) == 0 {
		return errors.Join(DatabaseError, fmt.Errorf("no leave of absence requests found for user %s", userID))
	}

	if len(storedLOAS) > 1 {
		return errors.Join(DatabaseError, fmt.Errorf("multiple leave of absence requests found for user %s (this should be impossible)", userID))
	}

	storedLOA := storedLOAS[0]
	storedLOA.IsApproved = approved
	storedLOA.IsProcessed = true
	storedLOA.ProcessedBy = processingUser

	err = instance.WithContext(ctxt).Model(LeaveOfAbsenseRequest{}).
		Where("id = ?", storedLOA.ID).
		Save(&storedLOA).Error
	if err != nil {
		err = errors.Join(DatabaseError, err)
	}

	return err
}

func (l *LeaveOfAbsenseRequests) MarkReturned(ctxt context.Context, instance *GuildInstance, userID string, processingUser *DiscordID) error {
	// ToDo: Implement this
	panic("implement me")
	return nil
}

var (
	leaveOfAbsenceRequests = new(LeaveOfAbsenseRequests)
)

var leaveOfAbsence = ButtonEventHandler{
	discord.NewPrimaryButton("Leave of Absence", leaveOfAbsenceCustomID),
	[]bot.EventListener{
		leaveOfAbsenceEventListener,
		leaveOfAbsenceModalEventListener,
		leaveOfAbsenceModalSubmissionEventListener,
		leaveOfAbsenceApprovalFlowHandler,
		leaveOfAbsenceDeniedInputListener,
		//loaListCommandListener,
		//bot.NewListenerFunc(loaClearCommandListener),
		//loaDenyModalListener,
	},
}

var leaveOfAbsenceEventListener = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
	var nickname string
	if event.Member() != nil && event.Member().Nick != nil {
		nickname = *event.Member().Nick
	} else {
		nickname = event.User().Username
	}
	userID := event.User().ID
	userName := event.User().Tag()

	discordIDForTriggerer := NewDiscordID(userID, userName, nickname)

	if event.Data.CustomID() == leaveOfAbsenceCustomID {
		err := event.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetEmbeds(discord.NewEmbedBuilder().
				SetTitle("Leave of Absence").
				SetColor(0x5765f2).
				SetDescription(leaveOfAbsenceDescription).
				Build()).
			AddActionRow(discord.NewPrimaryButton("Add Details & Submit", leaveOfAbsenceModalCustomID)).
			Build())

		if err != nil {
			slog.Error("error while creating LOA message",
				slog.Any("err", err),
				slog.String("triggered by", discordIDForTriggerer.UserID.String()),
				slog.String("submitted by", discordIDForTriggerer.UserID.String()),
			)
		}
	}
})

var leaveOfAbsenceModalEventListener = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
	var nickname string
	if event.Member() != nil && event.Member().Nick != nil {
		nickname = *event.Member().Nick
	} else {
		nickname = event.User().Username
	}
	userID := event.User().ID
	userName := event.User().Tag()

	discordIDForTriggerer := NewDiscordID(userID, userName, nickname)

	if event.Data.CustomID() == leaveOfAbsenceModalCustomID {
		err := event.Modal(
			discord.NewModalCreateBuilder().
				SetTitle("Leave of Absence").
				SetCustomID(leaveOfAbsenceModalSubmissionCustomID).
				AddActionRow(discord.NewShortTextInput("reason", "Reason").
					WithRequired(true)).
				AddActionRow(discord.NewShortTextInput("date", "Approx. Return Date").
					WithMaxLength(25).
					WithRequired(true)).
				Build(),
		)

		if err != nil {
			slog.Error("error while showing LOA modal",
				slog.Any("err", err),
				slog.String("triggered by", discordIDForTriggerer.UserID.String()),
				slog.String("submitted by", discordIDForTriggerer.UserID.String()),
			)
		}
	}
})

var leaveOfAbsenceModalSubmissionEventListener = bot.NewListenerFunc(func(event *events.ModalSubmitInteractionCreate) {
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

	if event.Data.CustomID == leaveOfAbsenceModalSubmissionCustomID {
		reason, _ := event.Data.TextInputComponent("reason")
		date, _ := event.Data.TextInputComponent("date")

		loa := LeaveOfAbsenseRequest{
			SubmittedBy: discordIDForTriggerer,
			Reason:      reason.Value,
			ReturnETA:   date.Value,
			SubmittedOn: time.Now(),
		}

		ctxt, cncl := context.WithTimeout(context.Background(), defaultContextTimeout)
		err := leaveOfAbsenceRequests.Add(ctxt, instance, loa)
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
				slog.Error("error while adding leave of absence request",
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
					SetContent(fmt.Sprintf(":pencil: __Leave of Absence Request__ (LOA) submitted by %s\n\t:ledger: Reason: \"%s\"\n\t:clock4: Approx. return date: \"%s\"",
						discordIDForTriggerer, loa.Reason, loa.ReturnETA,
					)).
					AddActionRow(
						discord.NewPrimaryButton("Approve", leaveOfAbsenceApproveButtonCustomID+discordIDForTriggerer.UserID.String()),
						discord.NewDangerButton("Deny", leaveOfAbsenceDenyButtonCustomID+discordIDForTriggerer.UserID.String()),
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
					SetContent(":white_check_mark: Your leave of absence request has been submitted. You will receive updates via DM.").
					Build(),
				)

				if err != nil {
					slog.Error("failed to update LoA approval modal message for user!",
						slog.Any("err", err),
						slog.String("triggered by", discordIDForTriggerer.UserID.String()),
						slog.String("submitted by", discordIDForTriggerer.UserID.String()),
					)
				}
			}
		}
	}
})

var leaveOfAbsenceApprovalFlowHandler = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
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

	if strings.HasPrefix(discordButtonCustomID, leaveOfAbsenceApproveButtonCustomID) {
		leaveOfAbsenceRequestForUserID := strings.TrimPrefix(discordButtonCustomID, leaveOfAbsenceApproveButtonCustomID)

		ctxt, cncl := context.WithTimeout(context.Background(), defaultContextTimeout)
		err := leaveOfAbsenceRequests.Process(ctxt, instance, leaveOfAbsenceRequestForUserID, discordIDForTriggerer, true)
		cncl()

		if err != nil {
			slog.Error("failed to process leave of absence request",
				slog.Any("err", err),
				slog.String("triggered by", discordIDForTriggerer.UserID.String()),
				slog.String("submitted by", leaveOfAbsenceRequestForUserID),
			)

			_ = event.UpdateMessage(discord.NewMessageUpdateBuilder().
				ClearEmbeds().
				ClearContainerComponents().
				SetContent(
					fmt.Sprintf("%s\n:bangbang: Error approving leave of absence request (triggered by %s): %s.",
						event.Message.Content,
						discordIDForTriggerer,
						err),
				).Build(),
			)

			return
		} else {
			slog.Info("approved leave of absence request",
				slog.String("triggered by", discordIDForTriggerer.UserID.String()),
				slog.String("submitted by", leaveOfAbsenceRequestForUserID),
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
				slog.Error("failed to update LoA approval s1 message for user!",
					slog.Any("err", err),
					slog.String("triggered by", discordIDForTriggerer.UserID.String()),
					slog.String("submitted by", leaveOfAbsenceRequestForUserID),
				)
			}
		}

		dmChannel, err := event.Client().Rest().CreateDMChannel(snowflake.MustParse(leaveOfAbsenceRequestForUserID))
		if err == nil {
			_, _ = event.Client().Rest().CreateMessage(dmChannel.ID(),
				discord.NewMessageCreateBuilder().
					SetContent(":white_check_mark: Your leave of absence request has been **approved**.").
					Build(),
			)
		}

		// Let's not delete messages
		// _ = event.Client().Rest().DeleteMessage(loaApprovalChannelID, event.Message.ID)

	} else if strings.HasPrefix(discordButtonCustomID, leaveOfAbsenceDenyButtonCustomID) {
		leaveOfAbsenceRequestForUserID := strings.TrimPrefix(discordButtonCustomID, leaveOfAbsenceDenyButtonCustomID)

		err := event.Modal(
			discord.NewModalCreateBuilder().
				SetTitle("Deny Leave Of Absence Request").
				SetCustomID(leaveOfAbsenceDenyModalCustomID + leaveOfAbsenceRequestForUserID).
				AddActionRow(discord.NewShortTextInput("deny-reason", "Reason for denial (required)")).
				Build(),
		)

		if err != nil {
			slog.Error("failed to show deny modal",
				slog.Any("err", err),
				slog.String("triggered by", discordIDForTriggerer.UserID.String()),
				slog.String("submitted by", leaveOfAbsenceRequestForUserID),
			)
		}
	}
})

var leaveOfAbsenceDeniedInputListener = bot.NewListenerFunc(func(event *events.ModalSubmitInteractionCreate) {
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

	if strings.HasPrefix(discordButtonCustomID, leaveOfAbsenceDenyModalCustomID) {
		leaveOfAbsenceRequestForUserID := strings.TrimPrefix(discordButtonCustomID, leaveOfAbsenceDenyModalCustomID)

		reason, _ := event.Data.TextInputComponent("deny-reason")
		ctxt, cncl := context.WithTimeout(context.Background(), defaultContextTimeout)
		err := leaveOfAbsenceRequests.Process(ctxt,
			instance,
			leaveOfAbsenceRequestForUserID,
			discordIDForTriggerer,
			false,
		)
		cncl()

		if err != nil {
			slog.Error("failed to process leave of absence request",
				slog.Any("err", err),
				slog.String("triggered by", discordIDForTriggerer.UserID.String()),
				slog.String("submitted by", leaveOfAbsenceRequestForUserID),
			)

			_ = event.UpdateMessage(discord.NewMessageUpdateBuilder().
				ClearEmbeds().
				ClearContainerComponents().
				SetContent(
					fmt.Sprintf("%s\n:bangbang: Error denying leave of absence request (triggered by %s): %s.",
						event.Message.Content,
						discordIDForTriggerer,
						err),
				).Build(),
			)

			return
		} else {
			slog.Info("denied leave of absence request",
				slog.String("triggered by", discordIDForTriggerer.UserID.String()),
				slog.String("submitted by", leaveOfAbsenceRequestForUserID),
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
				slog.Error("failed to update LoA approval s1 message for user!",
					slog.Any("err", err),
					slog.String("triggered by", discordIDForTriggerer.UserID.String()),
					slog.String("submitted by", leaveOfAbsenceRequestForUserID),
				)
			}
		}

		dmChannel, err := event.Client().Rest().CreateDMChannel(snowflake.MustParse(leaveOfAbsenceRequestForUserID))
		if err == nil {
			_, _ = event.Client().Rest().CreateMessage(dmChannel.ID(),
				discord.NewMessageCreateBuilder().
					SetContentf(":x: Your leave of absence request has been **denied** for \"%s\". %s",
						reason.Value,
						helpSuffix,
					).
					Build(),
			)
		}
		//
		//_ = event.CreateMessage(discord.NewMessageCreateBuilder().
		//	SetEphemeral(true).
		//	SetContent("LOA denied and user notified.").
		//	Build())
		//
		//_ = event.Client().Rest().DeleteMessage(loaApprovalChannelID, event.Message.ID)
	}
})

//
//var loaListCommandListener = bot.NewListenerFunc(func(event *events.ApplicationCommandInteractionCreate) {
//	if event.Data.CommandName() == "loa-list" {
//		leaveRequestsMutex.Lock()
//		defer leaveRequestsMutex.Unlock()
//		if len(leaveRequests) == 0 {
//			event.CreateMessage(discord.NewMessageCreateBuilder().
//				SetContent("No current LOAs.").
//				Build())
//			return
//		}
//
//		var sb strings.Builder
//		//var allNicknames []string
//		sb.WriteString("**Current Leaves of Absence:**\n")
//		for _, req := range leaveRequests {
//			//allNicknames = append(allNicknames, req.NickName)
//			status := "🕐 pending"
//			if req.Approved != nil {
//				if *req.Approved {
//					status = "✅ approved by " + req.ReviewedBy
//				} else {
//					event.CreateMessage(discord.NewMessageCreateBuilder().
//						SetContent("No current requests.").
//						Build())
//					return
//				}
//				sb.WriteString(fmt.Sprintf(
//					"\n• <@%s> until (approximate return date): **%s** — reason: **%s** (%s)\n",
//					req.UserID, req.ReturnETA, req.Reason, status,
//				))
//			}
//		}
//
//		//slog.Info("Nicknames on file", slog.Any("nicknames", allNicknames))
//
//		event.CreateMessage(discord.NewMessageCreateBuilder().
//			SetContent(sb.String()).
//			Build())
//	}
//})
//
//func loaClearCommandListener(event *events.ApplicationCommandInteractionCreate) {
//	if event.Data.CommandName() != "loa-clear" {
//		return
//	}
//
//	//slog.Info("LOA-CLEAR command triggered")
//
//	userNameToClear := ""
//
//	// Extract the nickname option if it's present
//	if data, ok := event.Data.(discord.SlashCommandInteractionData); ok {
//		//slog.Info("LOA-CLEAR options", slog.Any("options", data.Options))
//		for _, opt := range data.Options {
//			//slog.Info("LOA-CLEAR option", slog.String("name", opt.Name), slog.String("type", fmt.Sprintf("%v", opt.Type)), slog.Any("value", opt.Value))
//			if opt.Name == "nickname" && opt.Type == discord.ApplicationCommandOptionTypeString {
//				var extracted string
//				if err := json.Unmarshal(opt.Value, &extracted); err != nil {
//					slog.Error("Failed to unmarshal option value", slog.Any("error", err))
//				} else {
//					userNameToClear = strings.TrimSpace(extracted)
//				}
//				break
//			}
//		}
//	}
//
//	//slog.Info("Proceeding to LOA clear logic", slog.String("userNameToClear", userNameToClear))
//
//	leaveRequestsMutex.Lock()
//	defer leaveRequestsMutex.Unlock()
//
//	if userNameToClear != "" {
//		found := false
//		userInputLower := strings.ToLower(userNameToClear)
//
//		const minInputLen = 3
//		if len(userInputLower) < minInputLen {
//			event.CreateMessage(discord.NewMessageCreateBuilder().
//				SetContent(fmt.Sprintf("Please provide at least %d characters to clear an LOA.", minInputLen)).
//				Build())
//			return
//		}
//
//		for i, loa := range leaveRequests {
//			nickLower := strings.ToLower(loa.NickName)
//
//			// Check exact match on full nickname
//			if nickLower == userInputLower {
//				leaveRequests = append(leaveRequests[:i], leaveRequests[i+1:]...)
//				found = true
//				break
//			}
//
//			// Otherwise, fuzzy match on last word
//			parts := strings.Fields(loa.NickName)
//			if len(parts) > 0 {
//				lastWord := strings.ToLower(parts[len(parts)-1])
//				if strings.Contains(lastWord, userInputLower) {
//					leaveRequests = append(leaveRequests[:i], leaveRequests[i+1:]...)
//					found = true
//					break
//				}
//			}
//		}
//
//		if found {
//			event.CreateMessage(discord.NewMessageCreateBuilder().
//				SetContent(fmt.Sprintf("Cleared LOA for %s.", userNameToClear)).
//				Build())
//		} else {
//			event.CreateMessage(discord.NewMessageCreateBuilder().
//				SetContent(fmt.Sprintf("No LOA found for %s.", userNameToClear)).
//				Build())
//		}
//	} else {
//		event.CreateMessage(discord.NewMessageCreateBuilder().
//			SetContent("Please specify a nickname to clear.").
//			Build())
//	}
//}
//
//func updateLOAForumPost(client bot.Client) {
//	leaveRequestsMutex.Lock()
//	defer leaveRequestsMutex.Unlock()
//
//	var newEntries strings.Builder
//
//	for _, req := range leaveRequests {
//		if req.Approved == nil {
//			continue
//		}
//
//		// More stable and readable key
//		key := fmt.Sprintf("%s|%s|%s|%v", req.UserID, req.ReturnETA, req.Reason, req.Approved)
//
//		loaForumAddedMutex.Lock()
//		if loaForumAdded[key] {
//			loaForumAddedMutex.Unlock()
//			continue
//		}
//		loaForumAdded[key] = true
//		loaForumAddedMutex.Unlock()
//
//		status := "🕐 pending"
//		if *req.Approved {
//			status = "✅ approved by " + req.ReviewedBy
//		} else {
//			status = fmt.Sprintf("❌ denied by %s — Reason: **%s**", req.ReviewedBy, req.DenyReason)
//		}
//
//		newEntries.WriteString(fmt.Sprintf(
//			"\n• %s submitted LOA request at %s until: **%s** reason: **%s** (Status: %s)\n",
//			req.NickName,
//			req.Submitted.In(time.FixedZone("CST", -6*60*60)).Format("Mon, 02 Jan 2006 15:04 MST"),
//			req.ReturnETA,
//			req.Reason,
//			status,
//		))
//	}
//
//	if newEntries.Len() == 0 {
//		return // Nothing new to add
//	}
//
//	if loaForumMessageID == 0 {
//		// Create new summary post
//		msg, err := client.Rest().CreateMessage(loaForumThreadID,
//			discord.NewMessageCreateBuilder().
//				SetContent("**Leave of Absence Log:**\n"+newEntries.String()).
//				Build())
//		if err != nil {
//			slog.Error("failed to create LOA forum post", slog.Any("err", err))
//			return
//		}
//		loaForumMessageID = msg.ID
//	} else {
//		// Append to existing message
//		msg, err := client.Rest().GetMessage(loaForumThreadID, loaForumMessageID)
//		if err != nil {
//			slog.Error("failed to fetch LOA forum post", slog.Any("err", err))
//			return
//		}
//
//		newContent := msg.Content + newEntries.String()
//		if len(newContent) > 2000 {
//			newContent = newContent[:2000] // truncate to Discord limit
//		}
//
//		_, err = client.Rest().UpdateMessage(loaForumThreadID, loaForumMessageID,
//			discord.NewMessageUpdateBuilder().SetContent(newContent).Build())
//		if err != nil {
//			slog.Error("failed to update LOA forum post", slog.Any("err", err))
//		}
//	}
//}
