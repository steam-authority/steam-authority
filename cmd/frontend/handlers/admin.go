package handlers

import (
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Jleagle/go-durationfmt"
	"github.com/gamedb/gamedb/cmd/frontend/helpers/datatable"
	"github.com/gamedb/gamedb/cmd/frontend/helpers/geo"
	"github.com/gamedb/gamedb/cmd/frontend/helpers/session"
	"github.com/gamedb/gamedb/pkg/config"
	"github.com/gamedb/gamedb/pkg/helpers"
	"github.com/gamedb/gamedb/pkg/ldflags"
	"github.com/gamedb/gamedb/pkg/log"
	"github.com/gamedb/gamedb/pkg/memcache"
	"github.com/gamedb/gamedb/pkg/middleware"
	"github.com/gamedb/gamedb/pkg/mongo"
	"github.com/gamedb/gamedb/pkg/mysql"
	"github.com/gamedb/gamedb/pkg/oauth"
	"github.com/gamedb/gamedb/pkg/queue"
	"github.com/gamedb/gamedb/pkg/steam"
	"github.com/gamedb/gamedb/pkg/tasks"
	"github.com/gamedb/gamedb/pkg/websockets"
	"github.com/go-chi/chi"
	"go.mongodb.org/mongo-driver/bson"
	"go.uber.org/zap"
)

func AdminRouter() http.Handler {

	r := chi.NewRouter()

	r.Use(middleware.MiddlewareAuthCheck)
	r.Use(middleware.MiddlewareAdminCheck(Error404Handler))

	r.Get("/", adminHandler)
	r.Get("/consumers", adminConsumersHandler)
	r.Get("/consumers.json", adminConsumersAjaxHandler)
	r.Get("/queues", adminQueuesHandler)
	r.Get("/settings", adminSettingsHandler)
	r.Get("/stats", adminStatsHandler)
	r.Get("/tasks", adminTasksHandler)
	r.Get("/users", adminUsersHandler)
	r.Get("/users.json", adminUsersAjaxHandler)
	r.Get("/webhooks", adminWebhooksHandler)
	r.Get("/webhooks.json", adminWebhooksAjaxHandler)
	r.Get("/websockets", adminWebsocketsHandler)
	r.Post("/queues", adminQueuesHandler)
	r.Post("/settings", adminSettingsHandler)
	return r
}

func adminHandler(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/admin/stats", http.StatusFound)
}

func adminUsersHandler(w http.ResponseWriter, r *http.Request) {

	t := adminUsersTemplate{}
	t.fill(w, r, "admin_users", "Admin", "Admin")

	returnTemplate(w, r, t)
}

type adminUsersTemplate struct {
	globalTemplate
}

func adminUsersAjaxHandler(w http.ResponseWriter, r *http.Request) {

	var query = datatable.NewDataTableQuery(r, false)
	var wg sync.WaitGroup

	var users []mysql.User
	var playerIDs = map[int]map[oauth.ProviderEnum]string{}
	wg.Add(1)
	go func(r *http.Request) {

		defer wg.Done()

		db, err := mysql.GetMySQLClient()
		if err != nil {
			log.ErrS(err)
			return
		}

		db = db.Model(&mysql.User{})
		db = db.Select([]string{"id", "created_at", "email", "email_verified", "level", "logged_in_at"})
		db = db.Limit(100)
		db = db.Offset(query.GetOffset())

		sortCols := map[string]string{
			"1": "created_at",
			"2": "created_at",
			"4": "level",
		}
		db = query.SetOrderOffsetGorm(db, sortCols)

		db = db.Find(&users)
		if db.Error != nil {
			log.ErrS(db.Error)
			return
		}

		// Get Steam IDs
		db, err = mysql.GetMySQLClient()
		if err != nil {
			log.ErrS(err)
			return
		}

		var userIDs []int
		for _, v := range users {
			userIDs = append(userIDs, v.ID)
		}

		var userProviders []mysql.UserProvider
		db = db.Model(&mysql.UserProvider{})
		db = db.Where("user_id IN (?)", userIDs)

		db = db.Find(&userProviders)
		if db.Error != nil {
			log.ErrS(db.Error)
			return
		}

		for _, v := range userProviders {
			if _, ok := playerIDs[v.UserID]; !ok {
				playerIDs[v.UserID] = map[oauth.ProviderEnum]string{}
			}
			playerIDs[v.UserID][v.Provider] = v.ID
		}
	}(r)

	// Get total
	var count int64
	wg.Add(1)
	go func() {

		defer wg.Done()

		db, err := mysql.GetMySQLClient()
		if err != nil {
			log.ErrS(err)
			return
		}

		db = db.Table("users").Count(&count)
		if db.Error != nil {
			log.ErrS(db.Error)
			return
		}
	}()

	// Wait
	wg.Wait()

	var response = datatable.NewDataTablesResponse(r, query, count, count, nil)
	for _, user := range users {

		var createdAt = user.CreatedAt.Format(helpers.DateSQL)

		var loggedIn string
		if user.LoggedInAt != nil {
			loggedIn = user.LoggedInAt.Format(helpers.DateSQL)
		}

		response.AddRow([]interface{}{
			createdAt,          // 0
			user.Email,         // 1
			user.EmailVerified, // 2
			playerIDs[user.ID], // 3
			user.Level,         // 4
			loggedIn,           // 5
		})
	}

	returnJSON(w, r, response)
}

func adminConsumersAjaxHandler(w http.ResponseWriter, r *http.Request) {

	var query = datatable.NewDataTableQuery(r, false)
	var wg sync.WaitGroup
	var consumers []mysql.Consumer

	wg.Add(1)
	go func() {

		defer wg.Done()

		db, err := mysql.GetMySQLClient()
		if err != nil {
			log.ErrS(err)
			return
		}

		db = db.Model(&mysql.Consumer{})
		db = db.Select([]string{"expires", "owner", "environment", "version", "commits", "ip"})
		db = db.Limit(100)
		db = db.Offset(query.GetOffset())

		sortCols := map[string]string{
			"0": "expires",
			"4": "commits",
		}
		db = query.SetOrderOffsetGorm(db, sortCols)

		db = db.Find(&consumers)

		if db.Error != nil {
			log.ErrS(db.Error)
		}
	}()

	// Get total
	var count int64
	wg.Add(1)
	go func() {

		defer wg.Done()

		db, err := mysql.GetMySQLClient()
		if err != nil {
			log.ErrS(err)
			return
		}

		db = db.Table("consumers").Count(&count)
		if db.Error != nil {
			log.ErrS(db.Error)
			return
		}
	}()

	// Wait
	wg.Wait()

	var response = datatable.NewDataTablesResponse(r, query, count, count, nil)
	for _, consumer := range consumers {

		expires := consumer.Expires.Format(helpers.DateSQL)
		inDate := consumer.Expires.Add(mysql.ConsumerSessionLength).After(time.Now())

		response.AddRow([]interface{}{
			expires,              // 0
			consumer.Owner,       // 1
			consumer.Environment, // 2
			consumer.Version,     // 3
			consumer.Commits,     // 4
			consumer.IP,          // 5
			inDate,               // 6
		})
	}

	returnJSON(w, r, response)
}

func adminConsumersHandler(w http.ResponseWriter, r *http.Request) {

	t := adminConsumersTemplate{}
	t.fill(w, r, "admin_consumers", "Admin", "Admin")

	returnTemplate(w, r, t)
}

type adminConsumersTemplate struct {
	globalTemplate
}

func adminWebhooksHandler(w http.ResponseWriter, r *http.Request) {

	t := adminWebhooksTemplate{}
	t.fill(w, r, "admin_webhooks", "Admin", "Admin")
	t.addAssetChosen()

	services, err := mongo.GetDistict(mongo.CollectionWebhooks, "service")
	if err != nil {
		log.ErrS(err)
	} else {
		for _, v := range services {
			t.Services = append(t.Services, mongo.WebhookService(v.(string)))
		}
	}

	returnTemplate(w, r, t)
}

type adminWebhooksTemplate struct {
	globalTemplate
	Services []mongo.WebhookService
}

func adminWebhooksAjaxHandler(w http.ResponseWriter, r *http.Request) {

	query := datatable.NewDataTableQuery(r, false)

	var wg sync.WaitGroup

	filter := bson.D{}
	services := query.GetSearchSlice("service")
	if len(services) > 0 {
		filter = append(filter, bson.E{Key: "service", Value: bson.M{"$in": services}})
	}

	// Get webhooks
	var webhooks []mongo.Webhook
	wg.Add(1)
	go func() {

		defer wg.Done()

		var err error
		webhooks, err = mongo.GetWebhooks(query.GetOffset64(), 100, bson.D{{"created_at", -1}}, filter, nil)
		if err != nil {
			log.ErrS(err)
		}
	}()

	// Get count
	var count int64
	wg.Add(1)
	go func() {

		defer wg.Done()

		var err error
		count, err = mongo.CountDocuments(mongo.CollectionWebhooks, filter, 0)
		if err != nil {
			log.ErrS(err)
		}
	}()

	// Wait
	wg.Wait()

	var response = datatable.NewDataTablesResponse(r, query, count, count, nil)
	for _, app := range webhooks {

		response.AddRow([]interface{}{
			app.CreatedAt.Format(helpers.DateSQL), // 0
			app.Service.ToString(),                // 1
			app.Event,                             // 2
			app.RequestBody,                       // 3
			app.GetHash(),                         // 4
		})
	}

	returnJSON(w, r, response)
}

func adminStatsHandler(w http.ResponseWriter, r *http.Request) {

	t := adminStatsTemplate{}
	t.fill(w, r, "admin_stats", "Admin", "Admin")

	t.Commits = ldflags.CommitCount
	t.Hash = config.GetShortCommitHash()

	// Oldest player
	players, err := mongo.GetPlayers(0, 1, bson.D{{"updated_at", 1}}, helpers.LastUpdatedQuery, bson.M{"updated_at": 1})
	if err != nil {
		log.ErrS(err)
	}

	if len(players) > 0 {
		t.Oldest, err = durationfmt.Format(time.Now().Sub(players[0].UpdatedAt), "%d days")
		if err != nil {
			log.ErrS(err)
		}
	}

	t.Private, err = mongo.CountDocuments(mongo.CollectionPlayers, bson.D{{"community_visibility_state", 1}}, 0)
	if err != nil {
		log.ErrS(err)
	}

	t.Removed, err = mongo.CountDocuments(mongo.CollectionPlayers, bson.D{{"removed", true}}, 0)
	if err != nil {
		log.ErrS(err)
	}

	t.IP = r.RemoteAddr
	t.Cores = runtime.NumCPU()

	var location []string
	record, err := geo.GetLocation(r.RemoteAddr)
	if err == nil {
		if val, ok := record.Country.Names["en"]; ok {
			location = append(location, val)
		}
		if val, ok := record.City.Names["en"]; ok {
			location = append(location, val)
		}
	}

	t.Location = strings.Join(location, ", ")

	returnTemplate(w, r, t)
}

type adminStatsTemplate struct {
	globalTemplate
	Oldest   string
	Commits  string
	Hash     string
	Private  int64
	Removed  int64
	IP       string
	Location string
	Cores    int
}

func adminTasksHandler(w http.ResponseWriter, r *http.Request) {

	task := r.URL.Query().Get("run")
	if task != "" {

		c := r.URL.Query().Get("run")

		if val, ok := tasks.TaskRegister[c]; ok {
			go tasks.Run(val)
		}

		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)

		_, err := w.Write([]byte(http.StatusText(http.StatusOK)))
		if err != nil {
			log.ErrS(err)
		}

		return
	}

	//
	t := adminTasksTemplate{}
	t.fill(w, r, "admin_tasks", "Admin", "Admin")
	t.hideAds = true

	var grouped = map[tasks.TaskGroup][]adminTaskTemplate{}

	for _, v := range tasks.TaskRegister {
		grouped[v.Group()] = append(grouped[v.Group()], adminTaskTemplate{
			Task: v,
			Bad:  tasks.Bad(v),
			Next: tasks.Next(v),
			Prev: tasks.Prev(v),
		})
	}

	t.Tasks = []adminTaskListTemplate{
		{Tasks: grouped[tasks.TaskGroupApps], Title: "Apps"},
		{Tasks: grouped[tasks.TaskGroupBundles], Title: "Bundles"},
		{Tasks: grouped[tasks.TaskGroupPackages], Title: "Packages"},
		{Tasks: grouped[tasks.TaskGroupGroups], Title: "Groups"},
		{Tasks: grouped[tasks.TaskGroupPlayers], Title: "Players"},
		{Tasks: grouped[tasks.TaskGroupBadges], Title: "Badges"},
		{Tasks: grouped[tasks.TaskGroupNews], Title: "News"},
		{Tasks: grouped[tasks.TaskGroupElastic], Title: "Elastic"},
		{Tasks: grouped[""], Title: "Other"},
	}

	// Get configs for times
	configs, err := mysql.GetAllConfigs()
	if err != nil {
		log.ErrS(err)
	}

	t.Configs = configs

	returnTemplate(w, r, t)
}

type adminTasksTemplate struct {
	globalTemplate
	Tasks   []adminTaskListTemplate
	Configs map[string]mysql.Config
}

type adminTaskListTemplate struct {
	Title string
	Tasks []adminTaskTemplate
}

type adminTaskTemplate struct {
	Task tasks.TaskInterface
	Bad  bool
	Next time.Time
	Prev time.Time
}

func adminSettingsHandler(w http.ResponseWriter, r *http.Request) {

	if r.Method == http.MethodPost {

		err := r.ParseForm()
		if err != nil {
			log.ErrS(err)
		}

		middleware.DownMessage = r.PostFormValue("down-message")

		mcItem := r.PostFormValue("del-mc-item")
		if mcItem != "" {
			err := memcache.Delete(mcItem)
			if err != nil {
				log.ErrS(err)
			}
		}

		session.SetFlash(r, session.SessionGood, "Done")
		session.Save(w, r)

		http.Redirect(w, r, "/admin/settings", http.StatusFound)
		return
	}

	t := adminSettingsTemplate{}
	t.fill(w, r, "admin_settings", "Admin", "Admin")
	t.DownMessage = middleware.DownMessage

	returnTemplate(w, r, t)
}

type adminSettingsTemplate struct {
	globalTemplate
	DownMessage string
}

func adminQueuesHandler(w http.ResponseWriter, r *http.Request) {

	if r.Method == http.MethodPost {

		err := r.ParseForm()
		if err != nil {
			log.ErrS(err)
		}

		ua := r.UserAgent()

		// Apps
		var appIDs []int
		if val := r.PostForm.Get("app-id"); val != "" {

			vals := strings.Split(val, ",")

			for _, val := range vals {

				val = strings.TrimSpace(val)

				appID, err := strconv.Atoi(val)
				if err == nil {
					appIDs = append(appIDs, appID)
				}
			}
		}

		// Apps since timestamp
		if val := r.PostForm.Get("apps-ts"); val != "" {

			log.InfoS("Queueing apps")

			val = strings.TrimSpace(val)

			ts, err := strconv.ParseInt(val, 10, 64)
			if err == nil {

				apps, err := steam.GetSteam().GetAppList(100000, 0, ts, "")
				err = steam.AllowSteamCodes(err)
				if err != nil {
					log.ErrS(err)
				} else {

					log.InfoS("Found " + strconv.Itoa(len(apps.Apps)) + " apps")

					for _, app := range apps.Apps {
						appIDs = append(appIDs, app.AppID)
					}
				}
			}
		}

		// Packages
		var packageIDs []int
		if val := r.PostForm.Get("package-id"); val != "" {

			vals := strings.Split(val, ",")

			for _, val := range vals {

				val = strings.TrimSpace(val)

				packageID, err := strconv.Atoi(val)
				if err == nil {
					packageIDs = append(packageIDs, packageID)
				}
			}
		}

		// Players
		if val := r.PostForm.Get("player-id"); val != "" {

			vals := strings.Split(val, ",")

			for _, val := range vals {

				val = strings.TrimSpace(val)

				playerID, err := strconv.ParseInt(val, 10, 64)
				if err == nil {
					err = queue.ProducePlayer(queue.PlayerMessage{ID: playerID, UserAgent: &ua}, "frontend-admin")
					err = helpers.IgnoreErrors(err, memcache.ErrInQueue)
					if err != nil {
						log.Err(err.Error(), zap.Int64("id", playerID))
					}
				}
			}
		}

		// Players search
		if val := r.PostForm.Get("player-id-search"); val != "" {

			for _, val := range strings.Split(val, ",") {

				val = strings.TrimSpace(val)

				playerID, err := strconv.ParseInt(val, 10, 64)
				if err == nil {
					err = queue.ProducePlayerSearch(nil, playerID)
					err = helpers.IgnoreErrors(err, memcache.ErrInQueue)
					if err != nil {
						log.Err("Producing player search", zap.Error(err), zap.Int64("id", playerID))
					}
				}
			}
		}

		// Bundles
		if val := r.PostForm.Get("bundle-id"); val != "" {

			vals := strings.Split(val, ",")

			for _, val := range vals {

				val = strings.TrimSpace(val)

				bundleID, err := strconv.Atoi(val)
				if err == nil {

					err = queue.ProduceBundle(bundleID)
					err = helpers.IgnoreErrors(err, memcache.ErrInQueue)
					if err != nil {
						log.Err(err.Error(), zap.Int("id", bundleID))
					}
				}
			}
		}

		// Test
		if val := r.PostForm.Get("test-id"); val != "" {

			val = strings.TrimSpace(val)
			count, err := strconv.Atoi(val)
			if err != nil {
				log.ErrS(err)
			}

			for i := 1; i <= count; i++ {

				err = queue.ProduceTest(i)
				err = helpers.IgnoreErrors(err, memcache.ErrInQueue)
				if err != nil {
					log.Err(err.Error(), zap.Int("id", i))
				}
			}
		}

		// Groups
		if val := r.PostForm.Get("group-id"); val != "" {

			vals := strings.Split(val, ",")

			for _, val := range vals {

				val = strings.TrimSpace(val)

				err := queue.ProduceGroup(queue.GroupMessage{ID: val, UserAgent: &ua})
				err = helpers.IgnoreErrors(err, queue.ErrIsBot, memcache.ErrInQueue)
				if err != nil {
					log.ErrS(err)
				}
			}
		}

		// Group members
		if val := r.PostForm.Get("group-members"); val != "" {

			vals := strings.Split(val, ",")
			for _, val := range vals {

				val = strings.TrimSpace(val)

				page := 1
				for {
					resp, b, err := steam.GetSteam().GetGroup(val, "", page)
					err = steam.AllowSteamCodes(err)
					if err != nil {
						log.Err("Steam group details api", zap.Error(err), zap.String("resp", string(b)))
						continue
					}

					for _, playerID := range resp.Members.SteamID64 {

						err = queue.ProducePlayer(queue.PlayerMessage{ID: int64(playerID), SkipExistingPlayer: true}, "frontend-admin-group")
						err = helpers.IgnoreErrors(err, memcache.ErrInQueue)
						if err != nil {
							log.ErrS(err)
						}
					}

					if resp.NextPageLink == "" {
						break
					}

					page++
				}
			}
		}

		//
		err = queue.ProduceSteam(queue.SteamMessage{AppIDs: appIDs, PackageIDs: packageIDs})
		if err != nil {
			log.Err(err.Error(), zap.Ints("app-ids", appIDs), zap.Ints("pack-ids", packageIDs))
		}

		session.SetFlash(r, session.SessionGood, "Done")
		session.Save(w, r)

		http.Redirect(w, r, "/admin/tasks", http.StatusFound)
		return
	}

	t := adminQueuesTemplate{}
	t.fill(w, r, "admin_queues", "Admin", "Admin")

	returnTemplate(w, r, t)
}

type adminQueuesTemplate struct {
	globalTemplate
}

func adminWebsocketsHandler(w http.ResponseWriter, r *http.Request) {

	t := adminWebsocketsTemplate{}
	t.fill(w, r, "admin_websockets", "Admin", "Admin")
	t.Websockets = websockets.Pages

	for _, v := range websockets.Pages {
		t.Total += v.CountConnections()
	}

	returnTemplate(w, r, t)
}

type adminWebsocketsTemplate struct {
	globalTemplate
	Websockets map[websockets.WebsocketPage]*websockets.Page
	Total      int
}
