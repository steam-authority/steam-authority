package db

import (
	"encoding/json"
	"errors"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/datastore"
	"github.com/Jleagle/steam-go/steam"
	"github.com/gosimple/slug"
	"github.com/steam-authority/steam-authority/helpers"
	"github.com/steam-authority/steam-authority/logger"
	"github.com/steam-authority/steam-authority/steami"
	"github.com/steam-authority/steam-authority/storage"
)

const maxBytesToStore = 1024 * 10

var (
	ErrInvalidPlayerID   = errors.New("invalid id")
	ErrInvalidPlayerName = errors.New("invalid name")

	cachePlayersCount int
)

type Player struct {
	CreatedAt        time.Time `datastore:"created_at"`               //
	UpdatedAt        time.Time `datastore:"updated_at"`               //
	FriendsAddedAt   time.Time `datastore:"friends_added_at,noindex"` //
	PlayerID         int64     `datastore:"player_id"`                //
	VanintyURL       string    `datastore:"vanity_url"`               //
	Avatar           string    `datastore:"avatar,noindex"`           //
	PersonaName      string    `datastore:"persona_name,noindex"`     //
	RealName         string    `datastore:"real_name,noindex"`        //
	CountryCode      string    `datastore:"country_code"`             //
	StateCode        string    `datastore:"status_code"`              //
	Level            int       `datastore:"level"`                    //
	GamesRecent      string    `datastore:"games_recent,noindex"`     // JSON
	GamesCount       int       `datastore:"games_count"`              //
	Badges           string    `datastore:"badges,noindex"`           // JSON
	BadgesCount      int       `datastore:"badges_count"`             //
	PlayTime         int       `datastore:"play_time"`                //
	TimeCreated      time.Time `datastore:"time_created"`             //
	LastLogOff       time.Time `datastore:"time_logged_off,noindex"`  //
	PrimaryClanID    int       `datastore:"primary_clan_id,noindex"`  //
	Friends          string    `datastore:"friends,noindex"`          // JSON
	FriendsCount     int       `datastore:"friends_count"`            //
	Donated          int       `datastore:"donated"`                  //
	Bans             string    `datastore:"bans"`                     // JSON
	NumberOfVACBans  int       `datastore:"bans_cav"`                 //
	NumberOfGameBans int       `datastore:"bans_game"`                //
	Groups           []int     `datastore:"groups,noindex"`           //
}

func (p Player) GetKey() (key *datastore.Key) {
	return datastore.NameKey(KindPlayer, strconv.FormatInt(p.PlayerID, 10), nil)
}

func (p Player) GetPath() string {

	x := "/players/" + strconv.FormatInt(p.PlayerID, 10)
	if p.PersonaName != "" {
		x = x + "/" + slug.Make(p.PersonaName)
	}
	return x
}

func (p Player) GetName() string {
	return p.PersonaName
}

func (p Player) GetSteamTimeUnix() (int64) {
	return p.TimeCreated.Unix()
}

func (p Player) GetSteamTimeNice() (string) {
	return p.TimeCreated.Format(helpers.DateYear)
}

func (p Player) GetLogoffUnix() (int64) {
	return p.LastLogOff.Unix()
}

func (p Player) GetLogoffNice() (string) {
	return p.LastLogOff.Format(helpers.DateYearTime)
}

func (p Player) GetUpdatedUnix() (int64) {
	return p.UpdatedAt.Unix()
}

func (p Player) GetUpdatedNice() (string) {
	return p.UpdatedAt.Format(helpers.DateTime)
}

func (p Player) GetSteamCommunityLink() string {
	return "https://steamcommunity.com/profiles/" + strconv.FormatInt(p.PlayerID, 10)
}

func (p Player) GetAvatar() string {
	if strings.HasPrefix(p.Avatar, "http") {
		return p.Avatar
	} else if p.Avatar != "" {
		return "https://steamcdn-a.akamaihd.net/steamcommunity/public/images/avatars/" + p.Avatar
	} else {
		return p.GetDefaultAvatar()
	}
}

func (p Player) GetDefaultAvatar() string {
	return "/assets/img/no-player-image.jpg"
}

func (p Player) GetFlag() string {
	return "/assets/img/flags/" + strings.ToLower(p.CountryCode) + ".png"
}

func (p Player) GetCountry() string {
	return helpers.CountryCodeToName(p.CountryCode)
}

func (p Player) LoadApps(sort string, limit int) (apps []PlayerApp, err error) {

	if p.GamesCount == 0 {
		return apps, err
	}

	apps, err = GetPlayerApps(p.PlayerID, sort, limit)
	if err != nil {
		return apps, err
	}

	return apps, nil
}

func (p Player) GetBadges() (badges steam.BadgesResponse, err error) {

	var bytes []byte

	if strings.HasPrefix(p.Badges, "/") {
		bytes, err = storage.Download(storage.PathBadges(p.PlayerID))
		if err != nil {
			return badges, err
		}
	} else {
		bytes = []byte(p.Badges)
	}

	if len(bytes) > 0 {
		err = json.Unmarshal(bytes, &badges)
		if err != nil {
			if strings.Contains(err.Error(), "cannot unmarshal") {
				logger.Info(err.Error() + " - " + string(bytes))
			} else {
				logger.Error(err)
			}
			return badges, err
		}
	}

	return badges, nil
}

func (p Player) GetFriends() (friends steam.FriendsList, err error) {

	var bytes []byte

	if strings.HasPrefix(p.Friends, "/") {
		bytes, err = storage.Download(storage.PathFriends(p.PlayerID))
		if err != nil {
			return friends, err
		}
	} else {
		bytes = []byte(p.Friends)
	}

	if len(bytes) > 0 {
		err = json.Unmarshal(bytes, &friends)
		if err != nil {
			if strings.Contains(err.Error(), "cannot unmarshal") {
				logger.Info(err.Error() + " - " + string(bytes))
			} else {
				logger.Error(err)
			}
			return friends, err
		}
	}

	return friends, nil
}

func (p Player) GetRecentGames() (games []steam.RecentlyPlayedGame, err error) {

	if p.GamesRecent == "" {
		return
	}

	var bytes []byte

	if strings.HasPrefix(p.GamesRecent, "/") {
		bytes, err = storage.Download(storage.PathRecentGames(p.PlayerID))
		if err != nil {
			return games, err
		}
	} else {
		bytes = []byte(p.GamesRecent)
	}

	err = json.Unmarshal(bytes, &games)
	if err != nil {
		if strings.Contains(err.Error(), "cannot unmarshal") {
			logger.Info(err.Error() + " - " + string(bytes))
		} else {
			logger.Error(err)
		}
		return games, err
	}

	return games, nil
}

func (p Player) GetBans() (bans steam.GetPlayerBanResponse, err error) {

	bytes := []byte(p.Bans)

	if len(bytes) > 0 {
		err = json.Unmarshal(bytes, &bans)
		if err != nil {
			if strings.Contains(err.Error(), "cannot unmarshal") {
				logger.Info(err.Error() + " - " + string(bytes))
			} else {
				logger.Error(err)
			}
			return bans, err
		}
	}

	return bans, nil
}

func (p Player) ShouldUpdateFriends() bool {
	return p.FriendsAddedAt.Unix() < (time.Now().Unix() - int64(60*60*24*30))
}

func (p Player) GetTimeShort() (ret string) {
	return helpers.GetTimeShort(p.PlayTime, 2)
}

func (p Player) GetTimeLong() (ret string) {
	return helpers.GetTimeLong(p.PlayTime, 5)
}

func (p *Player) Update(userAgent string) (errs []error) {

	if !IsValidPlayerID(p.PlayerID) {
		return []error{ErrInvalidPlayerID}
	}

	if helpers.IsBot(userAgent) {
		return []error{}
	}

	if p.UpdatedAt.Unix() > (time.Now().Unix() - int64(60*60*24)) { // 1 Day
		return []error{}
	}

	// Get summary
	err := p.updateSummary()
	if err != nil {
		return []error{err}
	}

	// Async the rest
	var wg sync.WaitGroup

	// Get games
	wg.Add(1)
	go func(p *Player) {
		err = p.updateGames()
		if err != nil {
			errs = append(errs, err)
		}
		wg.Done()
	}(p)

	// Get recent games
	wg.Add(1)
	go func(p *Player) {
		err = p.updateRecentGames()
		if err != nil {
			errs = append(errs, err)
		}
		wg.Done()
	}(p)

	// Get badges
	wg.Add(1)
	go func(p *Player) {
		err = p.updateBades()
		if err != nil {
			errs = append(errs, err)
		}
		wg.Done()
	}(p)

	// Get friends
	wg.Add(1)
	go func(p *Player) {
		err = p.updateFriends()
		if err != nil {
			errs = append(errs, err)
		}
		wg.Done()
	}(p)

	// Get level
	wg.Add(1)
	go func(p *Player) {
		err = p.updateLevel()
		if err != nil {
			errs = append(errs, err)
		}
		wg.Done()
	}(p)

	// Get bans
	wg.Add(1)
	go func(p *Player) {
		err = p.updateBans()
		if err != nil {
			errs = append(errs, err)
		}
		wg.Done()
	}(p)

	// Get groups
	wg.Add(1)
	go func(p *Player) {
		err = p.updateGroups()
		if err != nil {
			errs = append(errs, err)
		}
		wg.Done()
	}(p)

	// Wait
	wg.Wait()

	err = p.Save()
	if err != nil {
		errs = append(errs, err)
	}

	return errs
}

func (p *Player) updateSummary() (error) {

	summary, _, err := steami.Steam().GetPlayer(p.PlayerID)
	if err != nil {
		return err
	}

	p.Avatar = strings.Replace(summary.AvatarFull, "https://steamcdn-a.akamaihd.net/steamcommunity/public/images/avatars/", "", 1)
	p.VanintyURL = path.Base(summary.ProfileURL)
	p.RealName = summary.RealName
	p.CountryCode = summary.LOCCountryCode
	p.StateCode = summary.LOCStateCode
	p.PersonaName = summary.PersonaName
	p.TimeCreated = time.Unix(summary.TimeCreated, 0)
	p.LastLogOff = time.Unix(summary.LastLogOff, 0)
	p.PrimaryClanID = summary.PrimaryClanID

	return err
}

func (p *Player) updateGames() (error) {

	resp, _, err := steami.Steam().GetOwnedGames(p.PlayerID)
	if err != nil {
		return err
	}

	// Loop apps
	var apps = map[int]*PlayerApp{}
	var appIDs []int
	var playtime = 0
	for _, v := range resp.Games {
		playtime = playtime + v.PlaytimeForever
		appIDs = append(appIDs, v.AppID)
		apps[v.AppID] = &PlayerApp{
			PlayerID:     p.PlayerID,
			AppID:        v.AppID,
			AppName:      v.Name,
			AppIcon:      v.ImgIconURL,
			AppTime:      v.PlaytimeForever,
			AppPrice:     0,
			AppPriceHour: 0,
		}
	}

	// Save data to player
	p.GamesCount = len(resp.Games)
	p.PlayTime = playtime

	// Go get price info from MySQL
	gamesSQL, err := GetApps(appIDs, []string{"id", "price_final"})
	logger.Error(err)

	for _, v := range gamesSQL {
		if v.PriceFinal > 0 {
			apps[v.ID].AppPrice = v.PriceFinal
			apps[v.ID].SetPriceHour()
		}
	}

	// Convert to slice
	var appsSlice []*PlayerApp
	for _, v := range apps {
		appsSlice = append(appsSlice, v)
	}

	err = BulkSavePlayerApps(appsSlice)
	logger.Error(err)

	return nil
}

func (p *Player) updateRecentGames() (error) {

	recentResponse, _, err := steami.Steam().GetRecentlyPlayedGames(p.PlayerID)
	if err != nil {
		return err
	}

	// Encode to JSON bytes
	bytes, err := json.Marshal(recentResponse.Games)
	if err != nil {
		return err
	}

	// Upload
	if len(bytes) > maxBytesToStore {
		storagePath := storage.PathRecentGames(p.PlayerID)
		err = storage.Upload(storagePath, bytes, false)
		if err != nil {
			return err
		}
		p.GamesRecent = storagePath
	} else {
		p.GamesRecent = string(bytes)
	}

	return nil
}

func (p *Player) updateBades() (error) {

	badgesResponse, _, err := steami.Steam().GetBadges(p.PlayerID)
	if err != nil {
		return err
	}

	p.BadgesCount = len(badgesResponse.Badges)

	// Encode to JSON bytes
	bytes, err := json.Marshal(badgesResponse)
	if err != nil {
		return err
	}

	// Upload
	if len(bytes) > maxBytesToStore {
		storagePath := storage.PathBadges(p.PlayerID)
		err = storage.Upload(storagePath, bytes, false)
		if err != nil {
			return err
		}
		p.Badges = storagePath
	} else {
		p.Badges = string(bytes)
	}

	return nil
}

func (p *Player) updateFriends() (error) {

	resp, _, err := steami.Steam().GetFriendList(p.PlayerID)
	if err != nil {
		return err
	}

	p.FriendsCount = len(resp.Friends)

	// Encode to JSON bytes
	bytes, err := json.Marshal(resp)
	if err != nil {
		return err
	}

	// Upload
	if len(bytes) > maxBytesToStore {
		storagePath := storage.PathFriends(p.PlayerID)
		err = storage.Upload(storagePath, bytes, false)
		if err != nil {
			return err
		}
		p.Friends = storagePath
	} else {
		p.Friends = string(bytes)
	}

	return nil
}

func (p *Player) updateLevel() (error) {

	level, _, err := steami.Steam().GetSteamLevel(p.PlayerID)
	if err != nil {
		return err
	}

	p.Level = level

	return nil
}

func (p *Player) updateBans() (error) {

	bans, _, err := steami.Steam().GetPlayerBans(p.PlayerID)
	if err == steam.ErrNoUserFound {
		return nil
	} else if err != nil {
		return err
	}

	p.NumberOfGameBans = bans.NumberOfGameBans
	p.NumberOfVACBans = bans.NumberOfVACBans

	// Encode to JSON bytes
	bytes, err := json.Marshal(bans)
	if err != nil {
		return err
	}

	p.Bans = string(bytes)

	return nil
}

func (p *Player) updateGroups() (error) {

	resp, _, err := steami.Steam().GetUserGroupList(p.PlayerID)
	if err != nil {
		return err
	}

	p.Groups = resp.GetIDs()

	return nil
}

func (p *Player) Save() (err error) {

	if !IsValidPlayerID(p.PlayerID) {
		return ErrInvalidPlayerID
	}

	// Fix dates
	p.UpdatedAt = time.Now()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now()
	}

	_, err = SaveKind(p.GetKey(), p)
	if err != nil {
		return err
	}

	return nil
}

// todo, check this is acurate
func IsValidPlayerID(id int64) bool {

	if id < 10000000000000000 {
		return false
	}

	if len(strconv.FormatInt(id, 10)) != 17 {
		return false
	}

	return true
}

func GetPlayer(id int64) (ret Player, err error) {

	if !IsValidPlayerID(id) {
		return ret, ErrInvalidPlayerID
	}

	client, ctx, err := GetDSClient()
	if err != nil {
		return ret, err
	}

	key := datastore.NameKey(KindPlayer, strconv.FormatInt(id, 10), nil)

	player := Player{}
	player.PlayerID = id

	err = client.Get(ctx, key, &player)

	err = checkForMissingPlayerFields(err)
	if err != nil {
		return player, err
	}

	return player, nil
}

func GetPlayerByName(name string) (player Player, err error) {

	if len(name) == 0 {
		return player, ErrInvalidPlayerName
	}

	client, ctx, err := GetDSClient()
	if err != nil {
		return player, err
	}

	q := datastore.NewQuery(KindPlayer).Filter("vanity_url =", name).Limit(1)

	var players []Player

	_, err = client.GetAll(ctx, q, &players)

	err = checkForMissingPlayerFields(err)
	if err != nil {
		return player, err
	}

	// Return the first one
	if len(players) > 0 {
		return players[0], nil
	}

	return player, datastore.ErrNoSuchEntity
}

func GetPlayersByEmail(email string) (ret []Player, err error) {

	client, ctx, err := GetDSClient()
	if err != nil {
		return ret, err
	}

	q := datastore.NewQuery(KindPlayer).Filter("settings_email =", email).Limit(1)

	var players []Player

	_, err = client.GetAll(ctx, q, &players)

	err = checkForMissingPlayerFields(err)
	if err != nil {
		return ret, err
	}

	if len(players) == 0 {
		return ret, datastore.ErrNoSuchEntity
	}

	return players, nil
}

func GetAllPlayers(order string, limit int) (players []Player, err error) {

	client, ctx, err := GetDSClient()
	if err != nil {
		return players, err
	}

	q := datastore.NewQuery(KindPlayer).Order(order)

	if limit > 0 {
		q = q.Limit(limit)
	}

	_, err = client.GetAll(ctx, q, &players)

	err = checkForMissingPlayerFields(err)
	if err != nil {
		return players, err
	}

	return players, nil
}

func GetPlayersByIDs(ids []int64) (friends []Player, err error) {

	if len(ids) > 1000 {
		return friends, ErrorTooMany
	}

	client, ctx, err := GetDSClient()
	if err != nil {
		return friends, err
	}

	var keys []*datastore.Key
	for _, v := range ids {
		keys = append(keys, datastore.NameKey(KindPlayer, strconv.FormatInt(v, 10), nil))
	}

	friends = make([]Player, len(keys))
	err = client.GetMulti(ctx, keys, friends)

	if checkGetMultiErrors(err) != nil {
		return friends, err
	}

	return friends, nil
}

func checkGetMultiErrors(err error) error {

	if err != nil {

		if multiErr, ok := err.(datastore.MultiError); ok {

			for _, v := range multiErr {
				err2 := checkGetMultiErrors(v)
				if err2 != nil {
					return err2
				}
			}

		} else if err2, ok := err.(*datastore.ErrFieldMismatch); ok {

			err3 := checkForMissingPlayerFields(err2)
			if err3 != nil {
				return err3
			}

		} else if err.Error() == ErrNoSuchEntity.Error() {

			return nil

		} else {

			return err

		}
	}

	return nil
}

func CountPlayers() (count int, err error) {

	if cachePlayersCount == 0 {

		client, ctx, err := GetDSClient()
		if err != nil {
			return count, err
		}

		q := datastore.NewQuery(KindPlayer)
		cachePlayersCount, err = client.Count(ctx, q)
		if err != nil {
			return count, err
		}
	}

	return cachePlayersCount, nil
}

func checkForMissingPlayerFields(err error) error {

	if err == nil {
		return nil
	}

	if err2, ok := err.(*datastore.ErrFieldMismatch); ok {

		removedColumns := []string{
			"settings_email",
			"settings_password",
			"settings_alerts",
			"settings_hidden",
			"games",
		}

		if helpers.SliceHasString(removedColumns, err2.FieldName) {
			return nil
		}
	}

	return err
}
