package pages

import (
	"net/http"

	"github.com/gamedb/gamedb/pkg/helpers"
	"github.com/gamedb/gamedb/pkg/log"
	"github.com/gamedb/gamedb/pkg/sql"
	"github.com/gamedb/gamedb/pkg/tasks"
	"github.com/go-chi/chi"
)

func TagsRouter() http.Handler {

	r := chi.NewRouter()
	r.Get("/", statsTagsHandler)
	return r
}

func statsTagsHandler(w http.ResponseWriter, r *http.Request) {

	// Get config
	config, err := tasks.TaskRegister[tasks.Tags{}.ID()].GetTaskConfig()
	if err != nil {
		err = helpers.IgnoreErrors(err, sql.ErrRecordNotFound)
		log.Err(err, r)
	}

	// Get tags
	tags, err := sql.GetAllTags()
	if err != nil {
		returnErrorTemplate(w, r, errorTemplate{Code: 500, Message: "There was an issue retrieving the tags.", Error: err})
		return
	}

	code := helpers.GetProductCC(r)
	prices := map[int]string{}
	for _, v := range tags {
		price, err := v.GetMeanPrice(code)
		log.Err(err, r)
		prices[v.ID] = price
	}

	// Template
	t := statsTagsTemplate{}
	t.fill(w, r, "Tags", "Top Steam tags")
	t.Tags = tags
	t.Date = config.Value
	t.Prices = prices

	returnTemplate(w, r, "tags", t)
}

type statsTagsTemplate struct {
	GlobalTemplate
	Tags   []sql.Tag
	Date   string
	Prices map[int]string
}
