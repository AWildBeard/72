package perscom_events

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"
	"gorm.io/gorm"
)

// to use as template for other files replace SandC prefix with another and replace anything that is school or course with another name
const (
	sAndCRequestCustomID                  = "sandc"
	selectedCourseCustomID                = "course"
	selectedCourseAvailabilityModalSubmit = "availability-modal"
	sAndCRequestApprove                   = "sandc-approve"
	sAndCRequestDeny                      = "sandc-deny"
	sAndCDenyModal                        = "sandc-deny-modal"
)

//go:embed school_and_course_request_description.txt
var schoolAndCourseDescription string

type SchoolAndCourseRequest struct {
	gorm.Model
	SubmittedByID   snowflake.ID `gorm:"not null;index"` // Don't use, just for database compatibility
	SubmittedBy     *DiscordID   `gorm:"foreignKey:SubmittedByID;references:UserID" validate:"required"`
	SubmittedOn     time.Time    `validate:"required"`
	RequestedCourse string       `validate:"required"`
	Availability    string       `validate:"required"`
	IsApproved      bool
	IsProcessed     bool
	ProcessedByID   snowflake.ID `gorm:"index"`
	ProcessedBy     *DiscordID   `gorm:"foreignKey:ProcessedByID;references:UserID"`
}

type SchoolAndCourseRequests struct{}

func InitializeSchoolAndCourseRequestsFor(instance *GuildInstance) error {
	instance.Lock()
	defer instance.Unlock()

	err := instance.AutoMigrate(SchoolAndCourseRequest{})

	return err
}

// Add will accept a school and course request and add it to the database for the guild instance provided.
// Adding the request will update field data for the pointer received by Add so database-specific data can be
// retained and used by further callers
func (s *SchoolAndCourseRequests) Add(ctxt context.Context, instance *GuildInstance, sAndCRequest *SchoolAndCourseRequest) (err error) {
	if err := validate.Struct(sAndCRequest); err != nil {
		return errors.Join(ValidationError, err)
	}

	if sAndCRequest.ProcessedByID != 0 ||
		sAndCRequest.ProcessedBy != nil ||
		sAndCRequest.IsApproved ||
		sAndCRequest.IsProcessed {
		return fmt.Errorf("%w: a school and course request cannot be approved, denied, or processed before being added", ValidationError)
	}

	instance.Lock()
	defer instance.Unlock()

	count := int64(0)

	err = instance.WithContext(ctxt).Model(SchoolAndCourseRequest{}).
		Where("submitted_by_id = ? and is_processed = false and requested_course = ?", sAndCRequest.SubmittedByID, sAndCRequest.RequestedCourse).
		Count(&count).Error

	if err != nil {
		return errors.Join(DatabaseError, err)
	}

	if count > 0 {
		return fmt.Errorf("%w: a school and course request cannot be submitted more than once per user per course", UserReportableError)
	}

	err = instance.WithContext(ctxt).Model(SchoolAndCourseRequest{}).Create(sAndCRequest).Error

	if err != nil {
		return errors.Join(DatabaseError, err)
	}

	return nil
}

func (s *SchoolAndCourseRequests) Process(
	ctxt context.Context,
	instance *GuildInstance,
	recordId uint,
	processingUser *DiscordID,
	approved bool) (sAndC *SchoolAndCourseRequest, err error) {

	if err := validate.Struct(processingUser); err != nil {
		return nil, errors.Join(ValidationError, err)
	}

	if instance == nil {
		return nil, fmt.Errorf("%w: instance is nil", ValidationError)
	}

	if err := validate.Var(recordId, "required,numeric"); err != nil {
		return nil, errors.Join(ValidationError, err)
	}

	instance.Lock()
	defer instance.Unlock()

	var sAndCRequests = make([]SchoolAndCourseRequest, 0)
	err = instance.WithContext(ctxt).Model(SchoolAndCourseRequest{}).
		Where("id = ?", recordId).
		Find(sAndCRequests).Error

	if err != nil {
		return nil, errors.Join(DatabaseError, err)
	}

	if len(sAndCRequests) == 0 {
		return nil, errors.Join(DatabaseError, fmt.Errorf("no school and course requests found for recordId %d", recordId))
	}

	if len(sAndCRequests) > 1 {
		return nil, errors.Join(DatabaseError, fmt.Errorf("multiple matching school and course requests found for recordId %d (this shouldn't be possible)", recordId))
	}

	storedSandCRequest := sAndCRequests[0]
	storedSandCRequest.IsApproved = approved
	storedSandCRequest.IsProcessed = true
	storedSandCRequest.ProcessedBy = processingUser

	err = instance.WithContext(ctxt).Model(SchoolAndCourseRequest{}).
		Where("id = ?", storedSandCRequest.ID).
		Save(&storedSandCRequest).Error

	if err != nil {
		err = errors.Join(DatabaseError, err)
	}

	return &storedSandCRequest, err
}

var schoolAndCourseRequests = new(SchoolAndCourseRequests)

var schoolAndCourseRequest = ButtonEventHandler{
	discord.NewPrimaryButton("Schools & Courses", sAndCRequestCustomID),
	[]bot.EventListener{
		sAndCEventListener,
		sAndCRequestSelectionEventListener,
		sAndCModalSubmitEventListener,
		sAndCApprovalFlowHandler,
		sAndCDenyModalSubmitEventListener,
		//SandCModalSubmissionEventListener,
		//SandCListCommandListener,
		//bot.NewListenerFunc(SandCClearCommandListener),
		//SandCApprovalButtonListener,
		//SandCDenyModalListener,
	},
}

var sAndCEventListener = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
	if event.Data.CustomID() == sAndCRequestCustomID {
		err := event.CreateMessage(
			discord.NewMessageCreateBuilder().
				SetEphemeral(true).
				SetEmbeds(discord.NewEmbedBuilder().
					SetTitle("School & Course Descriptions").
					SetDescription(schoolAndCourseDescription).
					SetColor(0x5765f2). // Example color
					Build()).
				AddActionRow(discord.NewStringSelectMenu(selectedCourseCustomID, "Select a school or course",
					discord.NewStringSelectMenuOption("Airborne", "Airborne"),
					discord.NewStringSelectMenuOption("Air Assault", "Air Assault"),
					discord.NewStringSelectMenuOption("Advanced Infantry Training", "Advanced Infantry Training"),
					discord.NewStringSelectMenuOption("Ranger School", "Ranger School"),
					discord.NewStringSelectMenuOption("Combat Life Saver", "Combat Life Saver"),
					discord.NewStringSelectMenuOption("Drill Instructor Course", "Drill Instructor Course"),
					discord.NewStringSelectMenuOption("NCO Training & Leadership", "NCO Training & Leadership"),
					discord.NewStringSelectMenuOption("Squad Designated Marksman (SDM)", "Squad Designated Marksman (SDM)"),
					discord.NewStringSelectMenuOption("Explosive Ordnance Disposal (EOD)", "Explosive Ordnance Disposal (EOD)"),
					discord.NewStringSelectMenuOption("Electronic Warfare Specialist (EWAR)", "Electronic Warfare Specialist (EWAR)"),
				)).
				Build(),
		)

		if err != nil {
			slog.Error("error while creating message", slog.Any("err", err))
		}
	}
})

var sAndCRequestSelectionEventListener = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
	if event.Data.CustomID() == selectedCourseCustomID {
		err := event.Modal(discord.NewModalCreateBuilder().
			SetTitle("Your Availability").
			SetCustomID(selectedCourseAvailabilityModalSubmit + event.StringSelectMenuInteractionData().Values[0]).
			AddActionRow(discord.NewShortTextInput("availability", "Availability")).
			Build(),
		)

		if err != nil {
			slog.Error("error while updating message", slog.Any("err", err))
		}
	}
})

var sAndCModalSubmitEventListener = bot.NewListenerFunc(func(event *events.ModalSubmitInteractionCreate) {
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

	if strings.HasPrefix(event.Data.CustomID, selectedCourseAvailabilityModalSubmit) {
		availability, _ := event.Data.TextInputComponent("availability")
		sAndCRequest := &SchoolAndCourseRequest{
			SubmittedBy:     discordIDForTriggerer,
			SubmittedOn:     time.Now().UTC(),
			RequestedCourse: strings.TrimPrefix(event.Data.CustomID, selectedCourseAvailabilityModalSubmit),
			Availability:    availability.Value,
		}

		ctxt, cncl := context.WithTimeout(context.Background(), defaultContextTimeout)
		// Adding the request will update field data for the pointer we sent
		err := schoolAndCourseRequests.Add(ctxt, instance, sAndCRequest)
		cncl()

		if err != nil {
			if errors.Is(err, UserReportableError) {
				_ = event.UpdateMessage(discord.NewMessageUpdateBuilder().
					ClearEmbeds().
					ClearContainerComponents().
					SetContent(fmt.Sprintf(":bangbang: %s. %s", err, helpSuffix)).
					Build(),
				)
			} else {
				slog.Error("error while adding school and course request",
					slog.Any("err", err),
					slog.String("triggered by", discordIDForTriggerer.UserID.String()),
					slog.String("submitted by", discordIDForTriggerer.UserID.String()),
				)

				_ = event.UpdateMessage(discord.NewMessageUpdateBuilder().
					ClearEmbeds().
					ClearContainerComponents().
					SetContent(fmt.Sprintf(":bangbang: I encountered an internal error. %s", helpSuffix)).
					Build(),
				)
			}
		} else {
			_, err = event.Client().Rest().CreateMessage(
				instance.SubmissionChannelID,
				discord.NewMessageCreateBuilder().
					SetContentf(":pencil: __School and Course Request__ (S&C) submitted by %s\n\t:books: Course: %s\n\t:clock4: Availability: %s",
						discordIDForTriggerer, sAndCRequest.RequestedCourse, sAndCRequest.Availability).
					AddActionRow(
						discord.NewPrimaryButton("Approve",
							fmt.Sprintf("%s|%s|%d",
								sAndCRequestApprove,
								userID.String(),
								sAndCRequest.ID)),
						discord.NewDangerButton("Deny",
							fmt.Sprintf("%s|%s|%d",
								sAndCRequestDeny,
								userID.String(),
								sAndCRequest.ID)),
					).
					Build(),
			)

			if err != nil {
				slog.Error("error while creating approval message", slog.Any("err", err))

				_ = event.UpdateMessage(discord.NewMessageUpdateBuilder().
					ClearEmbeds().
					ClearContainerComponents().
					SetContentf(":bangbang: I encountered an internal error. %s", helpSuffix).
					Build())
			} else {
				err = event.UpdateMessage(discord.NewMessageUpdateBuilder().
					ClearEmbeds().
					ClearContainerComponents().
					SetContent(":white_check_mark: Your school and course request has been submitted. You will receive updates via DM.").
					Build())

				if err != nil {
					slog.Error("failed to update SandC approval modal message for user!",
						slog.Any("err", err),
						slog.String("triggered by", discordIDForTriggerer.UserID.String()),
						slog.String("submitted by", discordIDForTriggerer.UserID.String()),
					)
				}
			}
		}
	}
})

var sAndCApprovalFlowHandler = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
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

	discordEventCustomId := event.Data.CustomID()

	if strings.HasPrefix(discordEventCustomId, sAndCRequestApprove) {
		splitCustomID := strings.Split(discordEventCustomId, "|")
		if len(splitCustomID) != 3 {
			slog.Error("failed to parse custom ID for school and course request approval",
				slog.String("triggered by", discordIDForTriggerer.UserID.String()),
			)

			_ = event.UpdateMessage(discord.NewMessageUpdateBuilder().
				ClearEmbeds().
				ClearContainerComponents().
				SetContentf("%s\n:bangbang: Error approving school and course request (triggered by %s): failed to parse custom ID",
					event.Message.Content,
					discordIDForTriggerer).
				Build())

			return
		}

		sAndCForUserID := splitCustomID[1]
		var sAndCRecordID uint

		if recordId, err := strconv.ParseUint(splitCustomID[2], 10, 32); err != nil {
			slog.Error("failed to parse sAndCRecordID",
				slog.Any("err", err),
				slog.String("triggered by", discordIDForTriggerer.UserID.String()),
				slog.String("submitted by", sAndCForUserID),
			)

			_ = event.UpdateMessage(discord.NewMessageUpdateBuilder().
				ClearEmbeds().
				ClearContainerComponents().
				SetContentf("%s\n:bangbang: Error approving school and course request (triggered by %s): failed to parse %s as uint",
					event.Message.Content,
					discordIDForTriggerer,
					splitCustomID[2],
				).
				Build())

		} else {
			sAndCRecordID = uint(recordId)
		}

		ctxt, cncl := context.WithTimeout(context.Background(), defaultContextTimeout)
		sAndCRequest, err := schoolAndCourseRequests.Process(ctxt, instance, sAndCRecordID, discordIDForTriggerer, true)
		cncl()

		if err != nil {
			slog.Error("failed to process school and course request",
				slog.Any("err", err),
				slog.String("triggered by", discordIDForTriggerer.UserID.String()),
				slog.String("approved by", discordIDForTriggerer.UserID.String()),
				slog.String("submitted by", sAndCForUserID),
			)

			_ = event.UpdateMessage(discord.NewMessageUpdateBuilder().
				ClearEmbeds().
				ClearContainerComponents().
				SetContentf("%s\n:bangbang: Error approving school and course request (triggered by %s): %s",
					event.Message.Content,
					discordIDForTriggerer,
					err).
				Build())

			return
		} else {
			slog.Info("school and course request approved",
				slog.String("triggered by", discordIDForTriggerer.UserID.String()),
				slog.String("approved by", discordIDForTriggerer.UserID.String()),
				slog.String("submitted by", sAndCForUserID))

			err = event.UpdateMessage(discord.NewMessageUpdateBuilder().
				ClearEmbeds().
				ClearContainerComponents().
				SetContentf("%s\n:white_check_mark: **Approved** by %s",
					event.Message.Content,
					discordIDForTriggerer).
				Build())

			if err != nil {
				slog.Error("failed to update SandC approval s1 message for user!",
					slog.Any("err", err),
					slog.String("triggered by", discordIDForTriggerer.UserID.String()),
					slog.String("approved by", discordIDForTriggerer.UserID.String()),
					slog.String("submitted by", sAndCForUserID),
				)
			}
		}

		dmChannel, err := event.Client().Rest().CreateDMChannel(snowflake.MustParse(sAndCForUserID))
		if err == nil {
			_, _ = event.Client().Rest().CreateMessage(dmChannel.ID(),
				discord.NewMessageCreateBuilder().
					SetContentf(":white_check_mark: Your school and course request for \"%s\" has been **approved**.", sAndCRequest.RequestedCourse).
					Build(),
			)
		}
	} else if strings.HasPrefix(discordEventCustomId, sAndCRequestDeny) {
		splitCustomID := strings.Split(discordEventCustomId, "|")
		if len(splitCustomID) != 3 {
			slog.Error("failed to parse custom ID for school and course request approval",
				slog.String("triggered by", discordIDForTriggerer.UserID.String()),
			)

			_ = event.UpdateMessage(discord.NewMessageUpdateBuilder().
				ClearEmbeds().
				ClearContainerComponents().
				SetContentf("%s\n:bangbang: Error denying school and course request (triggered by %s): failed to parse custom ID",
					event.Message.Content,
					discordIDForTriggerer).
				Build())

			return
		}

		sAndCForUserID := splitCustomID[1]
		var sAndCRecordID uint

		if recordId, err := strconv.ParseUint(splitCustomID[2], 10, 32); err != nil {
			slog.Error("failed to parse sAndCRecordID",
				slog.Any("err", err),
				slog.String("triggered by", discordIDForTriggerer.UserID.String()),
				slog.String("submitted by", sAndCForUserID),
			)

			_ = event.UpdateMessage(discord.NewMessageUpdateBuilder().
				ClearEmbeds().
				ClearContainerComponents().
				SetContentf("%s\n:bangbang: Error denying school and course request (triggered by %s): failed to parse %s as uint",
					event.Message.Content,
					discordIDForTriggerer,
					splitCustomID[2],
				).
				Build())

		} else {
			sAndCRecordID = uint(recordId)
		}

		err := event.Modal(
			discord.NewModalCreateBuilder().
				SetTitle("Deny School and Course Request").
				SetCustomID(fmt.Sprintf("%s|%s|%d", sAndCDenyModal, sAndCForUserID, sAndCRecordID)).
				AddActionRow(discord.NewShortTextInput("deny-reason", "Reason for denial (required)")).
				Build(),
		)

		if err != nil {
			slog.Error("failed to show deny modal",
				slog.Any("err", err),
				slog.String("triggered by", discordIDForTriggerer.UserID.String()),
				slog.String("submitted by", sAndCForUserID),
			)
		}
	}
})

var sAndCDenyModalSubmitEventListener = bot.NewListenerFunc(func(event *events.ModalSubmitInteractionCreate) {
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

	discordEventCustomId := event.Data.CustomID

	if strings.HasPrefix(discordEventCustomId, sAndCDenyModal) {
		splitCustomID := strings.Split(discordEventCustomId, "|")
		if len(splitCustomID) != 3 {
			slog.Error("failed to parse custom ID for school and course request approval",
				slog.String("triggered by", discordIDForTriggerer.UserID.String()),
			)

			_ = event.UpdateMessage(discord.NewMessageUpdateBuilder().
				ClearEmbeds().
				ClearContainerComponents().
				SetContentf("%s\n:bangbang: Error denying school and course request (triggered by %s): failed to parse custom ID",
					event.Message.Content,
					discordIDForTriggerer).
				Build())

			return
		}

		sAndCForUserID := splitCustomID[1]
		var sAndCRecordID uint

		if recordId, err := strconv.ParseUint(splitCustomID[2], 10, 32); err != nil {
			slog.Error("failed to parse sAndCRecordID",
				slog.Any("err", err),
				slog.String("triggered by", discordIDForTriggerer.UserID.String()),
				slog.String("submitted by", sAndCForUserID),
			)

			_ = event.UpdateMessage(discord.NewMessageUpdateBuilder().
				ClearEmbeds().
				ClearContainerComponents().
				SetContentf("%s\n:bangbang: Error denying school and course request (triggered by %s): failed to parse %s as uint",
					event.Message.Content,
					discordIDForTriggerer,
					splitCustomID[2],
				).
				Build())

		} else {
			sAndCRecordID = uint(recordId)
		}

		denyReason, _ := event.Data.TextInputComponent("deny-reason")
		ctxt, cncl := context.WithTimeout(context.Background(), defaultContextTimeout)
		sAndCRequest, err := schoolAndCourseRequests.Process(ctxt,
			instance,
			sAndCRecordID,
			discordIDForTriggerer,
			false)
		cncl()

		if err != nil {
			slog.Error("failed to process school and course request",
				slog.Any("err", err),
				slog.String("triggered by", discordIDForTriggerer.UserID.String()),
				slog.String("submitted by", sAndCForUserID),
			)

			_ = event.UpdateMessage(discord.NewMessageUpdateBuilder().
				ClearEmbeds().
				ClearContainerComponents().
				SetContentf("%s\n:bangbang: Error denying school and course request (triggered by %s): %s",
					event.Message.Content,
					discordIDForTriggerer,
					err).
				Build())

			return
		} else {
			slog.Info("school and course request denied",
				slog.String("approved by", discordIDForTriggerer.UserID.String()),
				slog.String("submitted by", sAndCForUserID))

			err = event.UpdateMessage(discord.NewMessageUpdateBuilder().
				ClearEmbeds().
				ClearContainerComponents().
				SetContentf("%s\n:x: **Denied** by %s for \"%s\"",
					event.Message.Content,
					discordIDForTriggerer,
					denyReason.Value).
				Build())

			if err != nil {
				slog.Error("failed to update SandC approval s1 message for user!",
					slog.Any("err", err),
					slog.String("triggered by", discordIDForTriggerer.UserID.String()),
					slog.String("submitted by", sAndCForUserID),
				)
			}
		}

		dmChannel, err := event.Client().Rest().CreateDMChannel(snowflake.MustParse(sAndCForUserID))
		if err == nil {
			_, _ = event.Client().Rest().CreateMessage(dmChannel.ID(),
				discord.NewMessageCreateBuilder().
					SetContentf(":x: Your school and course request to attend \"%s\" has been **denied** for \"%s\". %s",
						sAndCRequest.RequestedCourse,
						denyReason.Value,
						helpSuffix,
					).
					Build(),
			)
		}
	}
})

//var SandCListCommandListener = bot.NewListenerFunc(func(event *events.ApplicationCommandInteractionCreate) {
//	if event.Data.CommandName() == "school-list" {
//		SandCRequestsMutex.Lock()
//		defer SandCRequestsMutex.Unlock()
//		if len(SandCRequests) == 0 {
//			event.CreateMessage(discord.NewMessageCreateBuilder().
//				SetContent("No current requests.").
//				Build())
//			return
//		}
//
//		var sb strings.Builder
//		//var allNicknames []string
//		sb.WriteString("**Current School and Course Requests:**\n")
//		for _, req := range SandCRequests {
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
//					"\n• %s submitted by **%s** at %s with availability of: **%s** (Status: %s)\n",
//					req.RequestedCourses,
//					req.Nickname,
//					req.Submitted.In(time.FixedZone("CST", -6*60*60)).Format("Mon, 02 Jan 2006 15:04 MST"),
//					req.Availability,
//					status,
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
//func SandCClearCommandListener(event *events.ApplicationCommandInteractionCreate) {
//	if event.Data.CommandName() != "school-clear" {
//		return
//	}
//
//	//slog.Info("SandC-CLEAR command triggered")
//
//	userNameToClear := ""
//
//	// Extract the nickname option if it's present
//	if data, ok := event.Data.(discord.SlashCommandInteractionData); ok {
//		//slog.Info("SandC-CLEAR options", slog.Any("options", data.Options))
//		for _, opt := range data.Options {
//			//slog.Info("SandC-CLEAR option", slog.String("name", opt.Name), slog.String("type", fmt.Sprintf("%v", opt.Type)), slog.Any("value", opt.Value))
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
//	//slog.Info("Proceeding to SandC clear logic", slog.String("userNameToClear", userNameToClear))
//
//	SandCRequestsMutex.Lock()
//	defer SandCRequestsMutex.Unlock()
//
//	if userNameToClear != "" {
//		found := false
//		userInputLower := strings.ToLower(userNameToClear)
//
//		const minInputLen = 3
//		if len(userInputLower) < minInputLen {
//			event.CreateMessage(discord.NewMessageCreateBuilder().
//				SetContent(fmt.Sprintf("Please provide at least %d characters to clear a request.", minInputLen)).
//				Build())
//			return
//		}
//
//		for i, SandC := range SandCRequests {
//			nickLower := strings.ToLower(SandC.Nickname)
//
//			// Check exact match on full nickname
//			if nickLower == userInputLower {
//				SandCRequests = append(SandCRequests[:i], SandCRequests[i+1:]...)
//				found = true
//				break
//			}
//
//			// Otherwise, fuzzy match on last word
//			parts := strings.Fields(SandC.Nickname)
//			if len(parts) > 0 {
//				lastWord := strings.ToLower(parts[len(parts)-1])
//				if strings.Contains(lastWord, userInputLower) {
//					SandCRequests = append(SandCRequests[:i], SandCRequests[i+1:]...)
//					found = true
//					break
//				}
//			}
//		}
//
//		if found {
//			event.CreateMessage(discord.NewMessageCreateBuilder().
//				SetContent(fmt.Sprintf("Cleared request for %s.", userNameToClear)).
//				Build())
//		} else {
//			event.CreateMessage(discord.NewMessageCreateBuilder().
//				SetContent(fmt.Sprintf("No request found for %s.", userNameToClear)).
//				Build())
//		}
//	} else {
//		event.CreateMessage(discord.NewMessageCreateBuilder().
//			SetContent("Please specify a nickname to clear.").
//			Build())
//	}
//}
//
//func updateSandCForumPost(client bot.Client) {
//	SandCRequestsMutex.Lock()
//	defer SandCRequestsMutex.Unlock()
//
//	var newEntries strings.Builder
//
//	for _, req := range SandCRequests {
//		if req.Approved == nil {
//			continue
//		}
//
//		// More stable and readable key
//		key := fmt.Sprintf("%s|%s|%v", req.UserID, req.Availability, req.Approved)
//
//		SandCForumAddedMutex.Lock()
//		if SandCForumAdded[key] {
//			SandCForumAddedMutex.Unlock()
//			continue
//		}
//		SandCForumAdded[key] = true
//		SandCForumAddedMutex.Unlock()
//
//		status := "🕐 pending"
//		if *req.Approved {
//			status = "✅ approved by " + req.ReviewedBy
//		} else {
//			status = fmt.Sprintf("❌ denied by %s — Reason: **%s**", req.ReviewedBy, req.DenyReason)
//		}
//
//		newEntries.WriteString(fmt.Sprintf(
//			"\n• %s submitted by **%s** at %s with availability of: **%s** (Status: %s)\n",
//			req.RequestedCourses,
//			req.Nickname,
//			req.Submitted.In(time.FixedZone("CST", -6*60*60)).Format("Mon, 02 Jan 2006 15:04 MST"),
//			req.Availability,
//			status,
//		))
//	}
//
//	if newEntries.Len() == 0 {
//		return // Nothing new to add
//	}
//
//	if SandCForumMessageID == 0 {
//		// Create new summary post
//		msg, err := client.Rest().CreateMessage(SandCForumThreadID,
//			discord.NewMessageCreateBuilder().
//				SetContent("**Schools and Courses Log:**\n"+newEntries.String()).
//				Build())
//		if err != nil {
//			slog.Error("failed to create SandC forum post", slog.Any("err", err))
//			return
//		}
//		SandCForumMessageID = msg.ID
//	} else {
//		// Append to existing message
//		msg, err := client.Rest().GetMessage(SandCForumThreadID, SandCForumMessageID)
//		if err != nil {
//			slog.Error("failed to fetch SandC forum post", slog.Any("err", err))
//			return
//		}
//
//		newContent := msg.Content + newEntries.String()
//		if len(newContent) > 2000 {
//			newContent = newContent[:2000] // truncate to Discord limit
//		}
//
//		_, err = client.Rest().UpdateMessage(SandCForumThreadID, SandCForumMessageID,
//			discord.NewMessageUpdateBuilder().SetContent(newContent).Build())
//		if err != nil {
//			slog.Error("failed to update SandC forum post", slog.Any("err", err))
//		}
//	}
//}
