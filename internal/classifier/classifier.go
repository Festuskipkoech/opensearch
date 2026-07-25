package classifier
 
import (
	"context"
	"fmt"
	"sort"
)

// Intent is the result of classifying a query.
type Intent struct {
	Class string // detected intent class
	Confidence float32  // cosine similarity score of the winning class
	Runner up string // second best class, used for hedge routing
	Vector []float32 // query embedding, reused by router and crawler
}

// Classifier classifies queries by comparing them to prototype vectors.
// Prototype vectors are computed once at startup from intentExamples.
