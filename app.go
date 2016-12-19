//go:generate go-bindata -o bindata.go templates/...

package main

import (
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alexflint/bufferlinks/buffer"
	arg "github.com/alexflint/go-arg"
	"github.com/alexflint/stdroots"
	"github.com/codegangsta/negroni"
	assetfs "github.com/elazarl/go-bindata-assetfs"
)

const buffertok = "b31/9a1c6e4de8e136b3c04c941233350e88a1"

func httpError(w http.ResponseWriter, format interface{}, parts ...interface{}) {
	http.Error(w, fmt.Sprintf(fmt.Sprintf("%v", format), parts...), http.StatusInternalServerError)
}

func mustParseTemplate(path string, filesystem bool) *template.Template {
	var buf []byte
	if filesystem {
		var err error
		buf, err = ioutil.ReadFile(path)
		if err != nil {
			panic(err)
		}
	} else {
		buf = MustAsset(path)
	}
	tpl, err := template.New(filepath.Base(path)).Parse(string(buf))
	if err != nil {
		panic(err)
	}
	return tpl
}

type app struct {
	store        *store
	lastFetch    []*article
	bufferClient *buffer.Client
	debug        bool
	profiles     []string // IDs of buffer profiles to post to
	indexTpl     *template.Template
	enqueueTpl   *template.Template
}

func (a *app) loadTemplates() {
	log.Println("loading templates...")
	a.indexTpl = mustParseTemplate("templates/index.html", a.debug)
	a.enqueueTpl = mustParseTemplate("templates/enqueue.html", a.debug)
}

func (a *app) refreshFeeds() error {
	var articles []*article

	// Marginal Revolution
	log.Println("polling marginal revolution...")
	mr, err := fetch("http://feeds.feedburner.com/marginalrevolution?fmt=xml")
	if err != nil {
		return err
	}
	for _, a := range mr {
		if strings.Contains(strings.ToLower(a.Title), "link") {
			articles = append(articles, a)
		}
	}

	// Slate Star Codex
	log.Println("polling slate star codex...")
	ssc, err := fetch("http://slatestarcodex.com/feed/")
	if err != nil {
		return err
	}
	for _, a := range ssc {
		if strings.Contains(strings.ToLower(a.Title), "link") {
			articles = append(articles, a)
		}
	}

	// foreXiv
	log.Println("polling forexiv...")
	forexiv, err := fetch("http://blog.jessriedel.com/feed/")
	if err != nil {
		return err
	}
	for _, a := range forexiv {
		if strings.Contains(strings.ToLower(a.Title), "link") {
			articles = append(articles, a)
		}
	}

	a.lastFetch = articles
	return nil
}

func (a *app) articles() ([]*article, error) {
	var filtered []*article
	for _, article := range a.lastFetch {
		state, err := a.store.findArticle(article.URL)
		if err != nil && err != errNotFound {
			return nil, fmt.Errorf("error while looking up article from %s in DB: %v", article.URL, err)
		}
		if state != nil && !state.DismissedAt.IsZero() {
			log.Printf("%s is dismissed", article.Title)
			continue
		}

		filtered = append(filtered, article)
		for _, link := range article.Links {
			state, err := a.store.findLink(link.URL)
			if err != nil && err != errNotFound {
				return nil, fmt.Errorf("error while looking up link from %s in DB: %v", article.URL, err)
			}
			if state != nil {
				link.Queued = true
				link.QueuedAt = state.QueuedAt
			}
		}
	}
	sort.Sort(sort.Reverse(byDate(filtered)))
	return filtered, nil
}

func (a *app) handleIndex(w http.ResponseWriter, r *http.Request) {
	if a.debug {
		a.loadTemplates()
	}

	articles, err := a.articles()
	if err != nil {
		httpError(w, err)
		return
	}

	err = a.indexTpl.Execute(w, map[string]interface{}{
		"Articles": articles,
	})
	if err != nil {
		httpError(w, err)
		return
	}
}

func (a *app) handleRefresh(w http.ResponseWriter, r *http.Request) {
	err := a.refreshFeeds()
	if err != nil {
		httpError(w, err)
		return
	}
	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
}

func (a *app) handleCommit(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		httpError(w, err)
		return
	}

	content := r.FormValue("content")
	url := r.FormValue("url")
	linkTitle := r.FormValue("link_title")
	linkDescr := r.FormValue("link_descr")

	_, err = a.bufferClient.CreateUpdate(a.profiles, buffer.UpdateOptions{
		Content:         content,
		LinkURL:         url,
		LinkTitle:       linkTitle,
		LinkDescription: linkDescr,
	})
	if err != nil {
		httpError(w, err)
		return
	}

	err = a.store.markLinkQueued(url)
	if err != nil {
		httpError(w, err)
		return
	}

	fmt.Fprintln(w, "pushed post to buffer")
}

func (a *app) handleEnqueue(w http.ResponseWriter, r *http.Request) {
	if a.debug {
		a.loadTemplates()
	}

	q := r.URL.Query()
	linkurl := q.Get("url")
	if linkurl == "" {
		http.Error(w, "url not provided", http.StatusBadRequest)
		return
	}

	err := a.enqueueTpl.Execute(w, map[string]interface{}{
		"URL": linkurl,
	})
	if err != nil {
		httpError(w, err.Error())
	}
}

func main() {
	var args struct {
		Debug bool
		DB    string
	}
	args.DB = "bufferlinks.boltdb"
	arg.MustParse(&args)

	port := os.Getenv("PORT")
	if port == "" {
		port = ":19870"
	}

	// Open DB
	store, err := newStore(args.DB)
	if err != nil {
		log.Fatal(err)
	}

	// Connect to Buffer
	client := buffer.NewClient(buffertok[2:len(buffertok)-2], stdroots.Client)
	profiles, err := client.Profiles()
	if err != nil {
		log.Fatal("error getting profiles: ", err)
	}
	var profileIDs []string
	for _, p := range profiles {
		if p.Service == "facebook" {
			profileIDs = append(profileIDs, p.Id)
		}
	}

	app := app{
		store:        store,
		bufferClient: client,
		profiles:     profileIDs,
		debug:        args.Debug,
	}
	app.loadTemplates()

	go func() {
		err := app.refreshFeeds()
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("fetched %d articles", len(app.lastFetch))
	}()

	// Set up static assets
	var static http.FileSystem
	if args.Debug {
		static = http.Dir("static")
	} else {
		static = &assetfs.AssetFS{Asset: Asset, AssetDir: AssetDir}
	}

	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(static)))
	http.HandleFunc("/enqueue", app.handleEnqueue)
	http.HandleFunc("/commit", app.handleCommit)
	http.HandleFunc("/", app.handleIndex)

	middleware := negroni.Classic()
	middleware.UseHandler(http.DefaultServeMux)

	log.Println("listening on", port)
	err = http.ListenAndServe(port, middleware)
	if err != nil {
		log.Fatal(err)
	}
}
