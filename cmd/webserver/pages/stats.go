package pages

import (
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/Jleagle/influxql"
	"github.com/dustin/go-humanize"
	"github.com/gamedb/gamedb/pkg/helpers"
	"github.com/gamedb/gamedb/pkg/log"
	"github.com/gamedb/gamedb/pkg/mongo"
	"github.com/gamedb/gamedb/pkg/sql"
	"github.com/go-chi/chi"
)

func StatsRouter() http.Handler {

	r := chi.NewRouter()
	r.Get("/", statsHandler)
	r.Get("/client-players.json", statsClientPlayersHandler)
	r.Get("/release-dates.json", statsDatesHandler)
	r.Get("/app-scores.json", statsScoresHandler)
	return r
}

func statsHandler(w http.ResponseWriter, r *http.Request) {

	// Template
	t := statsTemplate{}
	t.fill(w, r, "Stats", "Some interesting Steam Store stats.")
	t.addAssetHighCharts()

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {

		defer wg.Done()

		var err error
		t.PlayersCount, err = mongo.CountPlayers()
		log.Err(err, r)
	}()

	wg.Add(1)
	go func() {

		defer wg.Done()

		var err error
		t.AppsCount, err = sql.CountApps()
		log.Err(err, r)
	}()

	wg.Add(1)
	go func() {

		defer wg.Done()

		var err error
		t.BundlesCount, err = sql.CountBundles()
		log.Err(err, r)
	}()

	wg.Add(1)
	go func() {

		defer wg.Done()

		var err error
		t.PackagesCount, err = sql.CountPackages()
		log.Err(err, r)
	}()

	wg.Add(1)
	go func() {

		defer wg.Done()

		var err error

		a := sql.App{}
		t.OnlinePlayersCount, err = a.GetOnlinePlayers()
		log.Err(err, r)
	}()

	// Get total prices
	wg.Add(1)
	go func() {

		defer wg.Done()

		var err error
		var code = helpers.GetProductCC(r)
		var item = helpers.MemcacheStatsAppTypes(code)

		err = helpers.GetMemcache().GetSetInterface(item.Key, item.Expiration, &t.Totals, func() (interface{}, error) {

			var totals []statsAppTypeTotalsRow

			gorm, err := sql.GetMySQLClient()
			if err != nil {
				return totals, err
			}

			gorm = gorm.Select([]string{"type", "count(type) as count", "round(sum(JSON_EXTRACT(prices, \"$." + string(code) + ".final\"))) as total"})
			gorm = gorm.Table("apps")
			gorm = gorm.Group("type")
			gorm = gorm.Find(&totals)

			if gorm.Error != nil {
				log.Err(gorm.Error, r)
				return nil, gorm.Error
			}

			for k := range totals {

				app := sql.App{}
				app.Type = totals[k].Type
				totals[k].TypeFormatted = app.GetType()

				totals[k].CountFormatted = humanize.Comma(totals[k].Count)

				totals[k].TotalFormatted = helpers.FormatPrice(helpers.GetProdCC(code).CurrencyCode, int(math.Round(totals[k].Total)))
			}

			return totals, nil
		})
		if err != nil {
			log.Err(err)
			return
		}

		// Get total
		var total float64
		for _, v := range t.Totals {
			total += v.Total
		}

		t.TypeTotal = helpers.FormatPrice(helpers.GetProdCC(code).CurrencyCode, int(total))

	}()

	wg.Wait()

	returnTemplate(w, r, "stats", t)
}

type statsTemplate struct {
	GlobalTemplate
	AppsCount          int
	BundlesCount       int
	PackagesCount      int
	PlayersCount       int64
	OnlinePlayersCount int64
	Totals             []statsAppTypeTotalsRow
	TypeTotal          string
}

type statsAppTypeTotalsRow struct {
	Type           string  `gorm:"column:type"`
	Total          float64 `gorm:"column:total;type:float64"`
	Count          int64   `gorm:"column:count;type:int"`
	TypeFormatted  string  `gorm:"-"`
	TotalFormatted string  `gorm:"-"`
	CountFormatted string  `gorm:"-"`
}

func statsClientPlayersHandler(w http.ResponseWriter, r *http.Request) {

	builder := influxql.NewBuilder()
	builder.AddSelect("max(player_count)", "max_player_count")
	builder.AddSelect("max(player_online)", "max_player_online")
	builder.SetFrom(helpers.InfluxGameDB, helpers.InfluxRetentionPolicyAllTime.String(), helpers.InfluxMeasurementApps.String())
	builder.AddWhere("time", ">", "NOW() - 7d")
	builder.AddWhere("app_id", "=", "0")
	builder.AddGroupByTime("30m")
	builder.SetFillLinear()
	resp, err := helpers.InfluxQuery(builder.String())
	if err != nil {
		log.Err(err, r, builder.String())
		return
	}

	var hc helpers.HighChartsJson

	if len(resp.Results) > 0 && len(resp.Results[0].Series) > 0 {

		hc = helpers.InfluxResponseToHighCharts(resp.Results[0].Series[0])
	}

	returnJSON(w, r, hc)
}

func statsDatesHandler(w http.ResponseWriter, r *http.Request) {

	gorm, err := sql.GetMySQLClient()
	if err != nil {

		log.Err(err, r)
		return
	}

	var dates []statsAppReleaseDate

	gorm = gorm.Select([]string{"count(*) as count", "release_date_unix as date"})
	gorm = gorm.Table("apps")
	gorm = gorm.Group("date")
	gorm = gorm.Order("date desc")
	gorm = gorm.Where("release_date_unix > ?", time.Now().AddDate(-1, 0, 0).Unix())
	gorm = gorm.Where("release_date_unix < ?", time.Now().AddDate(0, 0, 1).Unix())
	gorm = gorm.Find(&dates)

	log.Err(gorm.Error, r)

	var ret [][]int64
	for _, v := range dates {
		ret = append(ret, []int64{v.Date * 1000, int64(v.Count)})
	}

	returnJSON(w, r, ret)
}

type statsAppReleaseDate struct {
	Date  int64
	Count int
}

func statsScoresHandler(w http.ResponseWriter, r *http.Request) {

	gorm, err := sql.GetMySQLClient()
	if err != nil {

		log.Err(err, r)
		return
	}

	var scores []statsAppScore

	gorm = gorm.Select([]string{"FLOOR(reviews_score) AS score", "count(reviews_score) AS count"})
	gorm = gorm.Table("apps")
	gorm = gorm.Where("reviews_score > ?", 0)
	gorm = gorm.Group("FLOOR(reviews_score)")
	gorm = gorm.Find(&scores)

	log.Err(gorm.Error, r)

	ret := make([]int, 101) // 0-100
	for i := 0; i <= 100; i++ {
		ret[i] = 0
	}
	for _, v := range scores {
		ret[v.Score] = v.Count
	}

	returnJSON(w, r, ret)
}

type statsAppScore struct {
	Score int
	Count int
}
