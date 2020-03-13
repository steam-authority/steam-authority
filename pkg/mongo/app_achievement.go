package mongo

import (
	"strconv"
	"strings"

	"github.com/gamedb/gamedb/pkg/helpers"
	"github.com/gamedb/gamedb/pkg/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type AppAchievement struct {
	AppID       int     `bson:"app_id" json:"-"`
	Key         string  `bson:"key" json:"-"`
	Name        string  `bson:"name" json:"name"`
	Description string  `bson:"description" json:"-"`
	Icon        string  `bson:"icon" json:"icon"`
	Completed   float64 `bson:"completed" json:"-"`
	Hidden      bool    `bson:"hidden" json:"-"`
	Active      bool    `bson:"active" json:"-"` // If it's part of the schema response
}

func (achievement AppAchievement) BSON() bson.D {

	return bson.D{
		{"app_id", achievement.AppID},
		{"key", achievement.Key},
		{"name", achievement.Name},
		{"description", achievement.Description},
		{"icon", achievement.Icon},
		{"completed", achievement.Completed},
		{"hidden", achievement.Hidden},
		{"active", achievement.Active},
	}
}

func (achievement AppAchievement) getKey() string {
	return strconv.Itoa(achievement.AppID) + "-" + achievement.Key
}

func (achievement AppAchievement) GetIcon() string {

	if !strings.HasPrefix(achievement.Icon, "/") && !strings.HasPrefix(achievement.Icon, "http") {
		achievement.Icon = helpers.AppIconBase + strconv.Itoa(achievement.AppID) + "/" + achievement.Icon
	}
	if !strings.HasSuffix(achievement.Icon, ".jpg") {
		achievement.Icon = achievement.Icon + ".jpg"
	}

	return achievement.Icon
}

func (achievement *AppAchievement) SetIcon(url string) {

	url = strings.TrimPrefix(url, helpers.AppIconBase+strconv.Itoa(achievement.AppID)+"/")
	url = strings.TrimSuffix(url, ".jpg")
	achievement.Icon = url
}

func GetAppAchievements(appID int, offset int64) (achievements []AppAchievement, err error) {

	var filter = bson.D{{
		"app_id", appID,
	}}

	cur, ctx, err := Find(CollectionAppAchievements, offset, 100, bson.D{{"completed", -1}}, filter, nil, nil)
	if err != nil {
		return achievements, err
	}

	defer func() {
		err = cur.Close(ctx)
		log.Err(err)
	}()

	for cur.Next(ctx) {

		var appAcievement AppAchievement
		err := cur.Decode(&appAcievement)
		if err != nil {
			log.Err(err, appAcievement.getKey())
		} else {
			achievements = append(achievements, appAcievement)
		}
	}

	return achievements, cur.Err()
}

func SaveAppAchievements(achievements []AppAchievement) (err error) {

	if len(achievements) == 0 {
		return nil
	}

	client, ctx, err := getMongo()
	if err != nil {
		return err
	}

	var writes []mongo.WriteModel
	for _, achievement := range achievements {

		write := mongo.NewReplaceOneModel()
		write.SetFilter(bson.M{"_id": achievement.getKey()})
		write.SetReplacement(achievement.BSON())
		write.SetUpsert(true)

		writes = append(writes, write)
	}

	c := client.Database(MongoDatabase).Collection(CollectionAppAchievements.String())

	_, err = c.BulkWrite(ctx, writes, options.BulkWrite())

	return err
}
