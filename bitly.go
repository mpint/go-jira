package jira

import (
	"strings"
	"github.com/mpint/go-bitly"
)

type OutputLinkMap struct {
	JiraLink bitly.Link
	StashLink bitly.Link
}

type InputLinkMap struct {
	name string
	url string
	urlTemplate string
}

type BitlyLink struct {
	index int
	name string
	link bitly.Link
}

func (c *Cli) ExtendLinksWithBitly(b *bitly.Client, data interface{}) (interface{}, error) {
	jiraLink := InputLinkMap{name: "jira", urlTemplate: "https://jira.cfops.it/browse/${issueName}"}
	stashLink := InputLinkMap{name: "stash", urlTemplate: "https://stash.cfops.it/projects/APPS/repos/app/browse?at=refs%2Fheads%2F${issueName}"}
	linksToExtend := []InputLinkMap{jiraLink, stashLink}
	jiraIssueKeyList := parseJiraLinks(data)
	bitlyLinkList, err := getBitlyLinks(b, jiraIssueKeyList, linksToExtend)
	extendedDataList := extendLinksWithBitlyUrls(data, bitlyLinkList)
	return extendedDataList, err
}

func getBitlyLink(b *bitly.Client, link InputLinkMap) (BitlyLink, error) {
	shortened, err := b.Links.Shorten(link.url)
	bitlyLink := BitlyLink{name: link.name, link: shortened}
	return bitlyLink, err
}

func getBitlyLinks(b *bitly.Client, jiraIssueList []string, linkList []InputLinkMap) ([]OutputLinkMap, error) {
	numLinks := len(linkList)
	numKeys := len(jiraIssueList)
	out := make([]OutputLinkMap, numKeys)
	channel := make(chan BitlyLink, numKeys)

	for _, link := range linkList {
		for i, issueName := range jiraIssueList {
			go func (i int, name string, link InputLinkMap) {
				link.url = strings.Replace(link.urlTemplate, "${issueName}", name, -1)

				bitlyLink, _ := getBitlyLink(b, link)
				bitlyLink.index = i

				channel <-bitlyLink
			}(i, issueName, link)
		}
	}

	for i := 0; i < numKeys * numLinks; i++ {
		bitlyLink := <-channel

		current := out[bitlyLink.index]

		if bitlyLink.name == "jira" {
			current.JiraLink = bitlyLink.link
			current.JiraLink.URL = current.JiraLink.URL[7:]
		} else if bitlyLink.name == "stash" {
			current.StashLink = bitlyLink.link
			current.StashLink.URL = current.StashLink.URL[7:]
		} else {
			panic("bad bitlyLink name")
		}

		out[bitlyLink.index] = current
	}

	return out, nil
}

func parseJiraLinks(data interface{}) []string {
	var out []string
	dat := data.(map[string]interface {})
	issueList := dat["issues"]
	issues := issueList.([]interface{})
	for _, v := range issues {
		issue := v.(map[string]interface {})
		issueName := issue["key"].(string)
		out = append(out, issueName)
	}

	return out
}

func extendLinksWithBitlyUrls(data interface{}, bitlyLinkList []OutputLinkMap) interface{} {
	dat := data.(map[string]interface {})
	issueList := dat["issues"]
	issues := issueList.([]interface{})
	for i, v := range issues {
		issue := v.(map[string]interface {})
		issue["bitlyLink"] = bitlyLinkList[i]
	}

	return data
}
