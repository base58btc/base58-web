package getters

import (
	"encoding/json"
	"net/http"
	"os"
)

// GetNotionData retrieves data from the Notion API
func GetNotionData() (NotionResponse, error) {
	// Set up the API endpoint and database ID
	endpoint := os.Getenv("NOTION_API_ENDPOINT")
	databaseID := os.Getenv("NOTION_DATABASE_ID")

	// Set up the HTTP client and request headers
	client := &http.Client{}
	req, err := http.NewRequest("POST", endpoint+databaseID+"/query", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Notion-Version", "2021-08-16")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+os.Getenv("NOTION_API_KEY"))

	// Send the request and parse the response
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var response NotionResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		return nil, err
	}

	return response, nil
}

// NotionResponse is the response from the Notion API
type NotionResponse struct {
	Object     string      `json:"object"`
	Results    []Page      `json:"results"`
	NextCursor interface{} `json:"next_cursor"`
	HasMore    bool        `json:"has_more"`
}

// Page is a struct that holds the data for a page
type Page struct {
	Object         string                 `json:"object"`
	ID             string                 `json:"id"`
	CreatedTime    string                 `json:"created_time"`
	LastEditedTime string                 `json:"last_edited_time"`
	CreatedBy      User                   `json:"created_by"`
	LastEditedBy   User                   `json:"last_edited_by"`
	Cover          *Cover                 `json:"cover"`
	Icon           *Icon                  `json:"icon"`
	Parent         Parent                 `json:"parent"`
	Archived       bool                   `json:"archived"`
	Properties     map[string]interface{} `json:"properties"`
	URL            string                 `json:"url"`
}

// User is a struct that holds the data for a user
type User struct {
	Object string `json:"object"`
	ID     string `json:"id"`
}

// Cover is a struct that holds the data for a cover
type Cover struct {
	Type  string `json:"type"`
	Value struct {
		External struct {
			URL string `json:"url"`
		} `json:"external"`
	} `json:"value"`
}

// Icon is a struct that holds the data for an icon
type Icon struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// Parent is a struct that holds the data for a parent
type Parent struct {
	Type        string `json:"type"`
	DatabaseID  string `json:"database_id,omitempty"`
	PageID      string `json:"page_id,omitempty"`
	WorkspaceID string `json:"workspace_id,omitempty"`
}
