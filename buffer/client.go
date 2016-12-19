package buffer

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
)

const (
	BufferURL = "https://api.bufferapp.com/1"
)

type Client struct {
	accessToken string
	http        *http.Client
}

func NewClient(accessToken string, http *http.Client) *Client {
	return &Client{
		accessToken: accessToken,
		http:        http,
	}
}

type Update struct {
	Id        string
	Text      string
	ProfileId string
}

type Profile struct {
	Avatar            string
	CreatedAt         int64
	Default           bool
	FormattedUsername string
	Id                string
	Schedules         []map[string][]string
	Service           string
	ServiceId         string
	ServiceUsername   string
	Statistics        map[string]interface{}
	TeamMembers       []string
	Timezone          string
	UserId            string
}

func (c *Client) Profiles() ([]Profile, error) {
	bufferResponse, err := c.get("profiles")
	if err != nil {
		return nil, err
	}

	var response []Profile
	err = json.Unmarshal(bufferResponse, &response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

type UpdateOptions struct {
	Content         string
	LinkURL         string
	LinkTitle       string
	LinkDescription string
}

func (c *Client) CreateUpdate(profileIds []string, opts UpdateOptions) ([]Update, error) {
	params := url.Values{}
	params.Set("text", opts.Content)
	for _, p := range profileIds {
		params.Add("profile_ids[]", p)
	}
	if opts.LinkURL != "" {
		params.Add("media[link]", opts.LinkURL)
		if opts.LinkTitle != "" {
			params.Add("media[title]", opts.LinkTitle)
		}
		if opts.LinkDescription != "" {
			params.Add("media[description]", opts.LinkDescription)
		}
	}

	bufferResponse, err := c.post("updates/create", params)
	if err != nil {
		return nil, err
	}

	var response struct {
		Success          bool
		BufferCount      int
		BufferPercentage int
		Updates          []Update
	}

	err = json.Unmarshal(bufferResponse, &response)
	if err != nil {
		return nil, err
	}

	if !response.Success {
		return nil, fmt.Errorf("buffer returned success=false: %v", response)
	}

	return response.Updates, nil
}

func (c *Client) get(resource string) ([]byte, error) {
	urlEndpoint := BufferURL + "/" + resource + ".json?access_token=" + c.accessToken
	resp, err := c.http.Get(urlEndpoint)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("buffer API said: %s", resp.Status)
	}
	return ioutil.ReadAll(resp.Body)
}

func (c *Client) post(resource string, params url.Values) ([]byte, error) {
	urlEndpoint := BufferURL + "/" + resource + ".json?access_token=" + c.accessToken
	resp, err := c.http.PostForm(urlEndpoint, params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("buffer API said: %s", resp.Status)
	}
	return ioutil.ReadAll(resp.Body)
}
