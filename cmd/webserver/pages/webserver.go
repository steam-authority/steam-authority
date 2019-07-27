package pages

import (
	"bytes"
	"encoding/json"
	"html"
	"html/template"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Jleagle/session-go/session"
	"github.com/Jleagle/steam-go/steam"
	"github.com/derekstavis/go-qs"
	"github.com/dustin/go-humanize"
	"github.com/gamedb/gamedb/pkg/config"
	"github.com/gamedb/gamedb/pkg/helpers"
	"github.com/gamedb/gamedb/pkg/log"
	"github.com/gamedb/gamedb/pkg/mongo"
	"github.com/gamedb/gamedb/pkg/sql"
	"github.com/go-chi/cors"
	"github.com/jinzhu/gorm"
	"github.com/justinas/nosurf"
	"github.com/mitchellh/mapstructure"
	"github.com/tdewolff/minify/v2"
	minhtml "github.com/tdewolff/minify/v2/html"
)

func setHeaders(w http.ResponseWriter, r *http.Request, contentType string) {

	csp := []string{
		"default-src 'none'",
		"script-src 'self' 'unsafe-eval' 'unsafe-inline' https://cdnjs.cloudflare.com https://cdn.datatables.net https://www.googletagmanager.com https://www.google-analytics.com https://connect.facebook.net https://platform.twitter.com https://www.google.com https://www.gstatic.com",
		"style-src 'self' 'unsafe-inline' https://cdnjs.cloudflare.com https://cdn.datatables.net https://fonts.googleapis.com",
		"media-src https://steamcdn-a.akamaihd.net",
		"font-src https://fonts.gstatic.com https://cdnjs.cloudflare.com",
		"frame-src https://platform.twitter.com https://staticxx.facebook.com https://www.facebook.com https://www.youtube.com https://www.google.com",
		"connect-src 'self' ws: wss:",
		"manifest-src 'self'",
	}

	if strings.HasPrefix(r.URL.Path, "/news") {
		csp = append(csp, "img-src 'self' data: *")
	} else {
		csp = append(csp, "img-src 'self' data: https://cdnjs.cloudflare.com https://steamcdn-a.akamaihd.net http://cdn.akamai.steamstatic.com https://steamcommunity-a.akamaihd.net https://www.google-analytics.com https://stats.g.doubleclick.net https://www.facebook.com https://www.google.com https://www.google.co.uk https://syndication.twitter.com https://cdn.discordapp.com")
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")                // MIME sniffing
	w.Header().Set("X-XSS-Protection", "1; mode=block")                // XSS
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")                    // Clickjacking
	w.Header().Set("Content-Security-Policy", strings.Join(csp, "; ")) // XSS
	w.Header().Set("Referrer-Policy", "no-referrer-when-downgrade")
	w.Header().Set("Feature-Policy", "geolocation 'none'; midi 'none'; sync-xhr 'none'; microphone 'none'; camera 'none'; magnetometer 'none'; gyroscope 'none'; speaker 'none'; fullscreen 'none'; payment 'none';")
	w.Header().Set("Server", "")
}

func returnJSON(w http.ResponseWriter, r *http.Request, i interface{}) (err error) {

	setHeaders(w, r, "application/json")

	b, err := json.Marshal(i)
	if err != nil {
		log.Err(err)
		return
	}

	_, err = w.Write(b)
	return err
}

func returnTemplate(w http.ResponseWriter, r *http.Request, page string, pageData interface{}) (err error) {

	// Set the last page
	if r.Method == "GET" && page != "error" {

		err = session.Set(r, helpers.SessionLastPage, r.URL.Path)
		if err != nil {
			log.Err(err, r)
		}
	}

	// Save the session
	err = session.Save(w, r)
	if err != nil {
		log.Err(err, r)
	}

	//
	setHeaders(w, r, "text/html")

	//
	folder := config.Config.TemplatesPath.Get()
	t, err := template.New("t").Funcs(getTemplateFuncMap()).ParseFiles(
		folder+"/_webpack_header.gohtml",
		folder+"/_webpack_footer.gohtml",
		folder+"/_header.gohtml",
		folder+"/_footer.gohtml",
		folder+"/_apps_header.gohtml",
		folder+"/_login_header.gohtml",
		folder+"/_flashes.gohtml",
		folder+"/_stats_header.gohtml",
		folder+"/_social.gohtml",
		folder+"/"+page+".gohtml",
	)
	if err != nil {
		log.Critical(err)
		return err
	}

	// Write a respone
	buf := &bytes.Buffer{}
	err = t.ExecuteTemplate(buf, page, pageData)
	if err != nil {
		log.Critical(err)
		return err
	}

	if config.IsProd() {

		m := minify.New()
		m.Add("text/html", &minhtml.Minifier{
			KeepConditionalComments: false,
			KeepDefaultAttrVals:     true,
			KeepDocumentTags:        true,
			KeepEndTags:             true,
			KeepWhitespace:          true,
		})

		err = m.Minify("text/html", w, buf)
		if err != nil {
			log.Err(err)
			return err
		}

	} else {

		_, err = buf.WriteTo(w)
	}

	log.Err(err)
	return err
}

func returnErrorTemplate(w http.ResponseWriter, r *http.Request, data errorTemplate) {

	if data.Title == "" {
		data.Title = "Error " + strconv.Itoa(data.Code)
	}

	if data.Code == 0 {
		data.Code = 500
	}

	log.Err(data.Error)

	data.fill(w, r, "Error", "Something has gone wrong!")

	w.WriteHeader(data.Code)

	err := returnTemplate(w, r, "error", data)
	log.Err(err, r)
}

type errorTemplate struct {
	GlobalTemplate
	Title   string
	Message string
	Code    int
	Error   error
	DataID  int64 // To add to the page attribute
}

func getTemplateFuncMap() map[string]interface{} {
	return template.FuncMap{
		"join":         func(a []string) string { return strings.Join(a, ", ") },
		"lower":        func(a string) string { return strings.ToLower(a) },
		"comma":        func(a int) string { return humanize.Comma(int64(a)) },
		"comma64":      func(a int64) string { return humanize.Comma(a) },
		"commaf":       func(a float64) string { return humanize.Commaf(a) },
		"bytes":        func(a uint64) string { return humanize.Bytes(a) },
		"seconds":      func(a int64) string { return humanize.RelTime(time.Now(), time.Now().Add(time.Second*time.Duration(a)), "", "") },
		"startsWith":   func(a string, b string) bool { return strings.HasPrefix(a, b) },
		"endsWith":     func(a string, b string) bool { return strings.HasSuffix(a, b) },
		"max":          func(a int, b int) float64 { return math.Max(float64(a), float64(b)) },
		"html":         func(html string) template.HTML { return helpers.RenderHTMLAndBBCode(html) },
		"json":         func(v interface{}) (string, error) { b, err := json.Marshal(v); log.Err(err); return string(b), err },
		"title":        func(a string) string { return strings.Title(a) },
		"inc":          func(i int) int { return i + 1 },
		"ordinalComma": func(i int) string { return helpers.OrdinalComma(i) },
		"https":        func(link string) string { return strings.Replace(link, "http://", "https://", 1) },
		"escape":       func(text string) string { return html.EscapeString(text) },
	}
}

// GlobalTemplate is added to every other template
type GlobalTemplate struct {
	Title       string        // Page title
	Description template.HTML // Page description
	Path        string        // URL path
	Env         string        // Environment
	CSSFiles    []Asset
	JSFiles     []Asset
	Canonical   string
	ProductCCs  []helpers.ProductCountryCode

	backgroundSet   bool
	Background      string
	BackgroundTitle string
	BackgroundLink  string

	FlashesGood []string
	FlashesBad  []string

	UserID        int
	UserName      string
	userEmail     string
	UserProductCC helpers.ProductCountryCode
	userLevel     string

	PlayerID   int64
	PlayerName string

	// contact page
	contactPage map[string]string

	// login page
	loginPage map[string]string

	// xp page
	PlayerLevel   int
	PlayerLevelTo int

	// Internal
	request   *http.Request
	metaImage string
	toasts    []Toast
	hideAds   bool
}

func (t *GlobalTemplate) fill(w http.ResponseWriter, r *http.Request, title string, description template.HTML) {

	var err error

	t.request = r

	t.Title = title + " - Game DB"
	t.Description = description
	t.Env = config.Config.Environment.Get()
	t.Path = r.URL.Path
	t.ProductCCs = helpers.GetProdCCs(true)

	val, err := session.Get(r, helpers.SessionUserID)
	log.Err(err, r)
	if val != "" {
		t.UserID, err = strconv.Atoi(val)
		log.Err(err, r)
	}

	val, err = session.Get(r, helpers.SessionPlayerID)
	log.Err(err, r)
	if val != "" {
		t.PlayerID, err = strconv.ParseInt(val, 10, 64)
		log.Err(err, r)
	}

	t.PlayerName, err = session.Get(r, helpers.SessionPlayerName)
	log.Err(err)

	t.userEmail, err = session.Get(r, helpers.SessionUserEmail)
	log.Err(err, r)

	t.userLevel, err = session.Get(r, helpers.SessionUserLevel)
	log.Err(err, r)

	t.UserName, err = session.Get(r, helpers.SessionPlayerName)
	log.Err(err, r)

	t.UserProductCC = helpers.GetProdCC(helpers.GetProductCC(r))
	log.Err(err, r)

	// Save country incase its from Maxmind
	err = session.Set(r, helpers.SessionUserProdCC, string(t.UserProductCC.ProductCode))
	log.Err(err)

	//
	t.setRandomBackground(true, false)

	// Pages
	switch true {
	case strings.HasPrefix(t.Path, "/contact"):

		// Details from form
		contactName, err := session.Get(r, "contact-name")
		log.Err(err)
		contactEmail, err := session.Get(r, "contact-email")
		log.Err(err)
		contactMessage, err := session.Get(r, "contact-message")
		log.Err(err)

		t.contactPage = map[string]string{
			"name":    contactName,
			"email":   contactEmail,
			"message": contactMessage,
		}

	case strings.HasPrefix(t.Path, "/login"):

		loginEmail, err := session.Get(r, "login-email")
		log.Err(err)

		t.loginPage = map[string]string{
			"email": loginEmail,
		}

	case strings.HasPrefix(t.Path, "/experience"):

		level, err := session.Get(r, helpers.SessionPlayerLevel)
		log.Err(err, r)

		if level == "" {
			t.PlayerLevel = 10
			t.PlayerLevelTo = 20
		} else {
			t.PlayerLevel, err = strconv.Atoi(level)
			log.Err(err, r)
			t.PlayerLevelTo = t.PlayerLevel + 10
		}
	}
}

func (t *GlobalTemplate) setBackground(app sql.App, title bool, link bool) {

	t.backgroundSet = true

	if app.Background != "" {
		t.Background = app.Background
	} else {
		return
	}
	if title {
		t.BackgroundTitle = app.GetName()
	} else {
		t.BackgroundTitle = ""
	}
	if link {
		t.BackgroundLink = app.GetPath()
	} else {
		t.BackgroundLink = ""
	}
}

func (t *GlobalTemplate) setRandomBackground(title bool, link bool) {

	if t.backgroundSet {
		return
	}

	if strings.HasPrefix(t.request.URL.Path, "/admin") {
		return
	}

	popularApps, err := sql.PopularApps()
	log.Err(err)

	if len(popularApps) > 0 {

		var i int
		for t.Background == "" {

			// Don't get stuck in the loop
			i++
			if i > 10 {
				break
			}

			backgroundApp := popularApps[rand.Intn(len(popularApps))]
			t.setBackground(backgroundApp, title, link)
		}
	}
}

func (t GlobalTemplate) GetUserJSON() string {

	stringMap := map[string]interface{}{
		"prodCC":             t.UserProductCC.ProductCode,
		"userCurrencySymbol": t.UserProductCC.Symbol,
		"toasts":             t.toasts,
		"log":                config.IsLocal() || t.IsAdmin(),
	}

	b, err := json.Marshal(stringMap)
	log.Err(err)

	return string(b)
}

func (t GlobalTemplate) GetMetaImage() (text string) {

	if t.metaImage == "" {
		return "https://gamedb.online/assets/img/sa-bg-500x500.png"
	}

	return t.metaImage
}

func (t GlobalTemplate) GetCanonical() (text string) {

	if t.Canonical != "" {
		return "https://gamedb.online" + t.Canonical
	}
	return "https://gamedb.online" + t.request.URL.Path + strings.TrimRight("?"+t.request.URL.Query().Encode(), "?")
}

func (t GlobalTemplate) GetVersionHash() string {

	if len(config.Config.CommitHash.Get()) >= 7 {
		return config.Config.CommitHash.Get()[0:7]
	}
	return ""
}

func (t GlobalTemplate) IsAppsPage() bool {
	return helpers.SliceHasString([]string{"apps", "upcoming", "new-releases", "trending", "packages", "bundles", "wishlists"}, strings.TrimPrefix(t.Path, "/"))
}

func (t GlobalTemplate) IsStatsPage() bool {
	return helpers.SliceHasString([]string{"stats", "tags", "genres", "publishers", "developers"}, strings.TrimPrefix(t.Path, "/"))
}

func (t GlobalTemplate) IsMorePage() bool {
	return helpers.SliceHasString([]string{"changes", "chat", "chat-bot", "contact", "coop", "experience", "info", "queues", "steam-api", "api"}, strings.TrimPrefix(t.Path, "/"))
}

func (t GlobalTemplate) IsSidebarPage() bool {
	return helpers.SliceHasString([]string{"api", "steam-api"}, strings.TrimPrefix(t.Path, "/"))
}

func (t GlobalTemplate) IsLoggedIn() bool {
	return t.UserID != 0
}

func (t GlobalTemplate) IsAdmin() bool {
	return isAdmin(t.request)
}

func (t GlobalTemplate) ShowAds() bool {

	if config.IsLocal() {
		return false
	}

	return !t.hideAds
}

func (t *GlobalTemplate) addToast(toast Toast) {
	t.toasts = append(t.toasts, toast)
}

func (t *GlobalTemplate) addAssetChosen() {
	t.JSFiles = append(t.JSFiles, Asset{URL: "https://cdnjs.cloudflare.com/ajax/libs/chosen/1.8.7/chosen.jquery.min.js", Integrity: "sha256-c4gVE6fn+JRKMRvqjoDp+tlG4laudNYrXI1GncbfAYY="})
}

func (t *GlobalTemplate) addAssetJSON2HTML() {
	t.JSFiles = append(t.JSFiles, Asset{URL: "https://cdnjs.cloudflare.com/ajax/libs/json2html/1.2.0/json2html.min.js", Integrity: "sha256-5iWhgkOOkWSQMxoIXqSKvZQHOTJ1wYDBqhMTFm5DkDw="})
	t.JSFiles = append(t.JSFiles, Asset{URL: "https://cdnjs.cloudflare.com/ajax/libs/jquery.json2html/1.2.0/jquery.json2html.min.js", Integrity: "sha256-NVPR5gsJCl/e6xUJ3Wv2+4Tui2vhZY6KBhx0RY0DNcs="})
}

func (t *GlobalTemplate) addAssetHighCharts() {
	t.JSFiles = append(t.JSFiles, Asset{URL: "https://cdnjs.cloudflare.com/ajax/libs/highcharts/7.0.1/highcharts.js", Integrity: "sha256-j3WPKr23emLOeDVvf5mbfGs5xE+GERqV1vCz+Wx6n74="})
	t.JSFiles = append(t.JSFiles, Asset{URL: "https://cdnjs.cloudflare.com/ajax/libs/highcharts/7.0.1/modules/data.js", Integrity: "sha256-CYgititANzm6qnx8M/4TpaGqfa8xFOIbHfWbtvKAg4w="})
}

func (t *GlobalTemplate) addAssetSlider() {
	t.JSFiles = append(t.JSFiles, Asset{URL: "https://cdnjs.cloudflare.com/ajax/libs/noUiSlider/12.1.0/nouislider.min.js", Integrity: "sha256-V76+FCDgnqVqafUQ74coiR7qA3Gd6ZlVuFgdwcGCGlc="})
	t.CSSFiles = append(t.CSSFiles, Asset{URL: "https://cdnjs.cloudflare.com/ajax/libs/noUiSlider/12.1.0/nouislider.min.css", Integrity: "sha256-MyPOSprr9/vRwXTYc0saw86ylzGM2HVRKWUfHIFta74="})
}

func (t *GlobalTemplate) addAssetCarousel() {
	t.JSFiles = append(t.JSFiles, Asset{URL: "https://cdnjs.cloudflare.com/ajax/libs/slick-carousel/1.9.0/slick.min.js", Integrity: "sha256-NXRS8qVcmZ3dOv3LziwznUHPegFhPZ1F/4inU7uC8h0="})
	t.CSSFiles = append(t.CSSFiles, Asset{URL: "https://cdnjs.cloudflare.com/ajax/libs/slick-carousel/1.9.0/slick.min.css", Integrity: "sha256-UK1EiopXIL+KVhfbFa8xrmAWPeBjMVdvYMYkTAEv/HI="})
	t.CSSFiles = append(t.CSSFiles, Asset{URL: "https://cdnjs.cloudflare.com/ajax/libs/slick-carousel/1.9.0/slick-theme.min.css", Integrity: "sha256-4hqlsNP9KM6+2eA8VUT0kk4RsMRTeS7QGHIM+MZ5sLY="})
}

func (t *GlobalTemplate) addAssetPasswordStrength() {
	t.JSFiles = append(t.JSFiles, Asset{URL: "https://cdnjs.cloudflare.com/ajax/libs/pwstrength-bootstrap/3.0.2/pwstrength-bootstrap.min.js", Integrity: "sha256-BPKP4P2AbrV7hf80SHJAJkIvjt7X7MKFEPpA99uU6uQ="})
}

func (t *GlobalTemplate) setFlashes(w http.ResponseWriter, r *http.Request) {

	var err error

	t.FlashesGood, err = session.GetFlashes(r, helpers.SessionGood)
	log.Err(err, r)

	t.FlashesBad, err = session.GetFlashes(r, helpers.SessionBad)
	log.Err(err, r)
}

//
type Asset struct {
	URL       string
	Integrity string
}

// Middleware
func middlewareAuthCheck() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			loggedIn, err := isLoggedIn(r)
			log.Err(err)

			if loggedIn && err == nil {
				next.ServeHTTP(w, r)
				return
			}

			err = session.SetFlash(r, helpers.SessionBad, "Please login")
			log.Err(err, r)

			http.Redirect(w, r, "/login", http.StatusFound)
			return
		})
	}
}

func middlewareAdminCheck() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			if isAdmin(r) {
				next.ServeHTTP(w, r)
				return
			}

			Error404Handler(w, r)
		})
	}
}

func middlewareCSRF(h http.Handler) http.Handler {

	ns := nosurf.New(h)
	ns.ExemptFunc(func(r *http.Request) bool {
		return !strings.Contains(r.URL.Path, "update.json")
	})

	return ns
}

//noinspection GoUnusedExportedFunction
func MiddlewareLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if config.IsLocal() {
			log.Info(log.LogNameRequests, r.Method+" "+r.URL.Path)
		}
		next.ServeHTTP(w, r)
	})
}

//noinspection GoUnusedExportedFunction
func MiddlewareTime(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		r.Header.Set("start-time", strconv.FormatInt(time.Now().UnixNano(), 10))

		next.ServeHTTP(w, r)
	})
}

// todo, check this is alright
func MiddlewareCors() func(next http.Handler) http.Handler {
	return cors.New(cors.Options{
		AllowedOrigins: []string{config.Config.GameDBDomain.Get()}, // Use this to allow specific origin hosts
		AllowedMethods: []string{"GET", "POST"},
	}).Handler
}

// DataTablesAjaxResponse
type DataTablesAjaxResponse struct {
	Draw            string          `json:"draw"`
	RecordsTotal    int64           `json:"recordsTotal,string"`
	RecordsFiltered int64           `json:"recordsFiltered,string"`
	Data            [][]interface{} `json:"data"`
	LevelLimited    bool            `json:"limited"`
}

func (t *DataTablesAjaxResponse) AddRow(row []interface{}) {
	t.Data = append(t.Data, row)
}

func (t DataTablesAjaxResponse) output(w http.ResponseWriter, r *http.Request) {

	if len(t.Data) == 0 {
		t.Data = make([][]interface{}, 0)
	}

	err := returnJSON(w, r, t)
	log.Err(err, r)
}

func (t *DataTablesAjaxResponse) limit(r *http.Request) {

	level := sql.UserLevel(helpers.GetUserLevel(r))
	max := level.MaxResults(100)

	if max > 0 && max < t.RecordsFiltered {
		t.RecordsFiltered = max
		t.LevelLimited = true
	}
}

// DataTablesQuery
type DataTablesQuery struct {
	Draw   string
	Order  map[string]map[string]interface{}
	Start  string
	Search map[string]interface{}
	// Time   string `mapstructure:"_"`
	// Columns []string
}

func (q *DataTablesQuery) fillFromURL(url url.Values) (err error) {

	// Convert string into map
	queryMap, err := qs.Unmarshal(url.Encode())
	if err != nil {
		return err
	}

	// Convert map into struct
	return mapstructure.Decode(queryMap, q)
}

func (q *DataTablesQuery) limit(r *http.Request) {

	level := sql.UserLevel(helpers.GetUserLevel(r))
	max := level.MaxOffset(100)

	start, _ := strconv.Atoi(q.Start)

	if max > 0 && int64(start) > max {
		q.Start = strconv.FormatInt(int64(start), 10)
	}
}

func (q DataTablesQuery) getSearchString(k string) (search string) {

	if val, ok := q.Search[k]; ok {
		if ok && val != "" {
			return val.(string)
		}
	}

	return ""
}

func (q DataTablesQuery) getSearchSlice(k string) (search []string) {

	if val, ok := q.Search[k]; ok {
		if val != "" {
			for _, v := range val.([]interface{}) {
				search = append(search, v.(string))
			}
		}
	}

	return search
}

func (q DataTablesQuery) getOrderSQL(columns map[string]string, code steam.ProductCC) (order string) {

	var ret []string

	for _, v := range q.Order {

		if col, ok := v["column"].(string); ok {
			if ok {

				if dir, ok := v["dir"].(string); ok {
					if ok {

						if col, ok := columns[col]; ok {
							if ok {

								if col == "price" {
									col = "JSON_EXTRACT(prices, \"$." + string(code) + ".final\")"
								}

								if dir == "asc" || dir == "desc" {
									ret = append(ret, col+" "+dir)
								}
							}
						}
					}
				}
			}
		}
	}

	return strings.Join(ret, ", ")
}

func (q DataTablesQuery) getOrderMongo(columns map[string]string, colEdit func(string) string) mongo.D {

	for _, v := range q.Order {

		if col, ok := v["column"].(string); ok {
			if ok {

				if dir, ok := v["dir"].(string); ok {
					if ok {

						if col, ok := columns[col]; ok {
							if ok {

								if colEdit != nil {
									col = colEdit(col)
								}

								if dir == "desc" {
									return mongo.D{{col, -1}}
								}

								return mongo.D{{col, 1}}
							}
						}
					}
				}
			}
		}
	}

	return mongo.D{}
}

func (q DataTablesQuery) getOrderString(columns map[string]string) (col string) {

	for _, v := range q.Order {

		if col, ok := v["column"].(string); ok {
			if ok {
				if col, ok := columns[col]; ok {
					if ok {
						return col
					}
				}
			}
		}
	}

	return col
}

func (q DataTablesQuery) setOrderOffsetGorm(db *gorm.DB, code steam.ProductCC, columns map[string]string) *gorm.DB {

	db = db.Order(q.getOrderSQL(columns, code))
	db = db.Offset(q.Start)

	return db
}

func (q DataTablesQuery) getOffset() int {
	i, _ := strconv.Atoi(q.Start)
	return i
}

func (q DataTablesQuery) getOffset64() int64 {
	i, _ := strconv.ParseInt(q.Start, 10, 64)
	return i
}

func (q DataTablesQuery) getPage(perPage int) int {

	i, _ := strconv.Atoi(q.Start)

	if i == 0 {
		return 1
	}

	return int(i/perPage) + 1
}

// Toasts
type Toast struct {
	Title   string `json:"title"`
	Message string `json:"message"`
	Link    string `json:"link"`
	Theme   string `json:"theme"`
	Timeout int    `json:"timeout"`
}

//
func isAdmin(r *http.Request) bool {

	id, err := session.Get(r, helpers.SessionUserID)
	log.Err(err)

	return id == "1"
}

func getUserFromSession(r *http.Request) (user sql.User, err error) {

	userID, err := helpers.GetUserIDFromSesion(r)
	if err != nil || userID == 0 {
		return user, err
	}

	return sql.GetUserByID(userID)
}

func isLoggedIn(r *http.Request) (val bool, err error) {
	read, err := session.Get(r, helpers.SessionUserEmail)
	return read != "", err
}
