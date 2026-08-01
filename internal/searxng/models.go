package searxng
 
// searchResponse mirrors the JSON structure SearXNG returns.
// Only fields we use are mapped — unknown fields are ignored.

type searchResponse struct {
	Results []searchResult `json:"results"`
}

type searchResult struct {
	URL string `json:"url"`
	Title string `json:"title"`
	Content string `json:"content"`
	Engine string `json:"engine"`
	Score float64 `json:"score"`
}