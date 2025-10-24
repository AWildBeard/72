package perscom_events

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
	"github.com/go-playground/validator/v10"
	"gorm.io/gorm"
)

var (
	defaultContextTimeout = 5 * time.Second
	UserReportableError   = errors.New("User error")
	DatabaseError         = errors.New("Database error")
	ValidationError       = errors.New("Validation error")
	validate              = validator.New(validator.WithRequiredStructEnabled())
	helpSuffix            = "If you need help, please seek help in the help channel!"
)

type DiscordID struct {
	UserID   snowflake.ID `gorm:"primaryKey" validate:"required"`
	Username string       `validate:"required"`
	Nickname string       `validate:"required"`
}

func NewDiscordID(discordID snowflake.ID, Username, Nickname string) *DiscordID {
	return &DiscordID{
		UserID:   discordID,
		Username: Username,
		Nickname: Nickname,
	}
}

func (d *DiscordID) String() string {
	return fmt.Sprintf("<@%s>", d.UserID)
}

type GuildInstance struct {
	*gorm.DB
	sync.RWMutex
	GuildID             snowflake.ID
	SubmissionChannelID snowflake.ID
}

type GuildInstances map[snowflake.ID]*GuildInstance

var guildInstances = make(GuildInstances)

type ButtonEventHandler struct {
	Button         discord.ButtonComponent
	EventListeners []bot.EventListener
}

var catalog = []ButtonEventHandler{
	temporaryPassRequest,
	leaveOfAbsence,
	schoolAndCourseRequest,
	blingBucksRequest,
	transferRequest,
	awardRecommendation,
	squadXML,
	dischargeRequest,
	sfasApplication,
}

func InitializeForNewGuild(setDb *gorm.DB, submissionChannelID snowflake.ID, guildID snowflake.ID) {
	newInstance := &GuildInstance{
		GuildID:             guildID,
		DB:                  setDb,
		RWMutex:             sync.RWMutex{},
		SubmissionChannelID: submissionChannelID,
	}

	guildInstances[guildID] = newInstance
	err := newInstance.AutoMigrate(&DiscordID{})
	if err != nil {
		panic(err)
	}

	err = InitializeTemporaryPassRequestsFor(newInstance)
	if err != nil {
		panic(err)
	}

	err = InitializeLeaveOfAbsenceRequestsFor(newInstance)
	if err != nil {
		panic(err)
	}

	err = InitializeSchoolAndCourseRequestsFor(newInstance)
	if err != nil {
		panic(err)
	}
}

func GetPublicEventHandlers() []ButtonEventHandler {
	return catalog
}

func GetCurrentOpTime() time.Time {
	today := time.Now().UTC()
	today = time.Date(today.Year(), today.Month(), today.Day(), 23, 0, 0, 0, time.UTC)
	offset := (6 - int(today.Weekday()) + 7) % 7 // Calculate days until next Saturday
	nextSaturday := today.AddDate(0, 0, offset)  // Add the offset to today's date to get next Saturday

	return nextSaturday
}
