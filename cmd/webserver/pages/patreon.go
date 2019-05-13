package pages

import (
	"net/http"
	"time"

	"github.com/Jleagle/patreon-go/patreon"
	"github.com/gamedb/gamedb/pkg/config"
	"github.com/gamedb/gamedb/pkg/log"
	"github.com/gamedb/gamedb/pkg/mongo"
	"github.com/go-chi/chi"
)

func PatreonRouter() http.Handler {
	r := chi.NewRouter()
	r.Post("/webhooks", patreonWebhookPostHandler)
	return r
}

func patreonWebhookPostHandler(w http.ResponseWriter, r *http.Request) {

	b, event, err := patreon.ValidateRequest(r, config.Config.PatreonSecret.Get())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Err(err)
		return
	}

	pwr, err := patreon.UnmarshalBytes(b)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Err(err)
		return
	}

	err = saveWebhookToMongo(event, pwr, b)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Err(err)
		return
	}

	err = saveWebhookEvent(r, event, pwr)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Err(err)
		return
	}
}

func saveWebhookToMongo(event string, pwr patreon.Webhook, body []byte) (err error) {

	_, err = mongo.InsertDocument(mongo.CollectionPatreonWebhooks, mongo.PatreonWebhook{
		CreatedAt:                   time.Now(),
		RequestBody:                 string(body),
		Event:                       event,
		UserID:                      int(pwr.User.ID),
		UserEmail:                   pwr.User.Attributes.Email,
		DataPatronStatus:            pwr.Data.Attributes.PatronStatus,
		DataLifetimeSupportCents:    pwr.Data.Attributes.LifetimeSupportCents,
		DataPledgeAmountCents:       pwr.Data.Attributes.PledgeAmountCents,
		DataPledgeCapAmountCents:    int(pwr.Data.Attributes.PledgeCapAmountCents),
		DataPledgeRelationshipStart: pwr.Data.Attributes.PledgeRelationshipStart,
	})
	return err
}

func saveWebhookEvent(r *http.Request, event string, pwr patreon.Webhook) (err error) {

	if pwr.User.Attributes.Email != "" {
		player := mongo.Player{}
		err = mongo.FindDocument(mongo.CollectionPlayers, "email", pwr.User.Attributes.Email, mongo.M{"_id": 1}, &player)
		if err == mongo.ErrNoDocuments {
			return nil
		}
		if err != nil {
			return err
		}

		return mongo.CreatePlayerEvent(r, player.ID, mongo.EventPatreon+"-"+event)
	}

	return nil
}
