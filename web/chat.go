package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"
	"github.com/cenkalti/backoff"
	"github.com/gamedb/website/config"
	"github.com/gamedb/website/log"
	"github.com/gamedb/website/websockets"
	"github.com/go-chi/chi"
)

const (
	guildID          = "407493776597057538"
	generalChannelID = "407493777058693121"
)

var (
	discordSession *discordgo.Session
)

// Called from main
func init() {

	// Retrying as this call can fail
	operation := func() (err error) {

		// Get client
		discordSession, err = discordgo.New("Bot " + config.Config.DiscordBotToken)
		if err != nil {
			return err
		}

		// Add websocket listener
		discordSession.AddHandler(discordMessageHandler)

		// Open connection
		return discordSession.Open()
	}

	policy := backoff.NewExponentialBackOff()

	err := backoff.Retry(operation, policy)
	if err != nil {
		log.Critical(err)
	}
}

func discordMessageHandler(s *discordgo.Session, m *discordgo.MessageCreate) {

	if m.Author.Bot {
		return
	}

	page, err := websockets.GetPage(websockets.PageChat)
	if err != nil {
		log.Err(err)
		return
	}

	if page.HasConnections() {

		page.Send(chatWebsocketPayload{
			AuthorID:     m.Author.ID,
			AuthorUser:   m.Author.Username,
			AuthorAvatar: m.Author.Avatar,
			Content:      m.Content,
			Channel:      m.ChannelID,
		})
	}
}

func chatRouter() http.Handler {
	r := chi.NewRouter()
	r.Get("/", chatHandler)
	r.Get("/{id}", chatHandler)
	r.Get("/{id}/ajax", chatAjaxHandler)
	return r
}

func chatHandler(w http.ResponseWriter, r *http.Request) {

	if discordSession == nil {
		returnErrorTemplate(w, r, errorTemplate{Code: 500, Message: "Could not connect to Discord."})
		return
	}

	// Get ID from URL
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Redirect(w, r, "/chat/"+generalChannelID, 302)
		return
	}

	// Template
	t := chatTemplate{}
	t.Fill(w, r, "Chat", "The Game DB community.")
	t.ChannelID = id
	t.addAssetJSON2HTML()

	//
	var wg sync.WaitGroup

	// Get channels
	wg.Add(1)
	go func() {

		defer wg.Done()

		channelsResponse, err := discordSession.GuildChannels(guildID)
		log.Err(err, r)

		for _, v := range channelsResponse {
			if v.Type == discordgo.ChannelTypeGuildText {

				// Fix channel name
				v.Name = strings.Replace(v.Name, "-", " ", 1)
				v.Name = strings.Replace(v.Name, "db", "DB", 1)
				v.Name = strings.Title(v.Name)

				t.Channels = append(t.Channels, v)
			}
		}

	}()

	// Get members
	wg.Add(1)
	go func() {

		defer wg.Done()

		membersResponse, err := discordSession.GuildMembers(guildID, "", 1000)
		log.Err(err, r)

		for _, v := range membersResponse {
			if !v.User.Bot {
				t.Members = append(t.Members, v)
			}
		}

	}()

	// Wait
	wg.Wait()

	err := returnTemplate(w, r, "chat", t)
	log.Err(err, r)
}

type chatTemplate struct {
	GlobalTemplate
	ChannelID string
	Channels  []*discordgo.Channel
	Members   []*discordgo.Member
}

func chatAjaxHandler(w http.ResponseWriter, r *http.Request) {

	setNoCacheHeaders(w)

	if discordSession == nil {
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		id = generalChannelID
	}

	messagesResponse, err := discordSession.ChannelMessages(id, 50, "", "", "")
	log.Err(err, r)

	var messages []chatWebsocketPayload
	for _, v := range messagesResponse {
		if !v.Author.Bot && v.Type == discordgo.MessageTypeDefault {

			messages = append(messages, chatWebsocketPayload{
				AuthorID:     v.Author.ID,
				AuthorUser:   v.Author.Username,
				AuthorAvatar: v.Author.Avatar,
				Content:      v.Content,
				Channel:      v.ChannelID,
			})
		}
	}

	bytes, err := json.Marshal(messages)
	log.Err(err, r)

	err = returnJSON(w, r, bytes)
	log.Err(err, r)
}

type chatWebsocketPayload struct {
	AuthorID     string `json:"author_id"`
	AuthorUser   string `json:"author_user"`
	AuthorAvatar string `json:"author_avatar"`
	Content      string `json:"content"`
	Channel      string `json:"channel"`
}
