package web

import (
	"net/http"
	"sync"

	"github.com/gamedb/website/db"
	"github.com/gamedb/website/logging"
)

func HomeHandler(w http.ResponseWriter, r *http.Request) {

	var wg sync.WaitGroup
	var err error

	var ranksCount int
	wg.Add(1)
	go func() {

		ranksCount, err = db.CountRanks()
		logging.Error(err)

		wg.Done()

	}()

	var appsCount int
	wg.Add(1)
	go func() {

		appsCount, err = db.CountApps()
		logging.Error(err)

		wg.Done()

	}()

	var packagesCount int
	wg.Add(1)
	go func() {

		packagesCount, err = db.CountPackages()
		logging.Error(err)

		wg.Done()

	}()

	wg.Wait()

	t := homeTemplate{}
	t.Fill(w, r, "Home")

	t.RanksCount = ranksCount
	t.AppsCount = appsCount
	t.PackagesCount = packagesCount

	returnTemplate(w, r, "home", t)
}

type homeTemplate struct {
	GlobalTemplate
	RanksCount    int
	AppsCount     int
	PackagesCount int
}
