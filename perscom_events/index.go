package perscom_events

import (
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
)

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

func GetButtonEventHandlers() []ButtonEventHandler {
	return catalog
}
