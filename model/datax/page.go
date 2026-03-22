package datax

import "strings"

// PagerInfo exposes paging input.
type PagerInfo interface {
	GetPageInfo() (pageSize, pageNumber int, pageToken string)
	SetPageNumber(pageNumber int)
}

// SortInfo exposes sorting input.
type SortInfo interface {
	GetSortInfo() []*SortPair
}

// SortAble stores raw sort input.
type SortAble struct {
	// OrderBy is in format "field[ desc], field2[ desc]".
	OrderBy string `json:"order_by" form:"order_by" auto_read:"order_by"`
}

// SortPair is a parsed sort field and direction.
type SortPair struct {
	// Field is the sortable field.
	Field        string
	// IsDescending indicates descending order.
	IsDescending bool
}

// GetSortInfo parses OrderBy into sort pairs.
func (s *SortAble) GetSortInfo() (paris []*SortPair) {
	if s.OrderBy == "" {
		return
	}

	sortFields := strings.Split(s.OrderBy, ",")
	for _, aField := range sortFields {
		nameAndDesc := strings.Split(strings.TrimSpace(aField), " ")
		p := &SortPair{Field: nameAndDesc[0]}
		if len(nameAndDesc) > 1 && nameAndDesc[1] == "desc" {
			p.IsDescending = true
		}
		paris = append(paris, p)
	}

	return
}
