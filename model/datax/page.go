package datax

import "strings"

type PagerInfo interface {
	GetPageInfo() (pageSize, pageNumber int, pageToken string)
	SetPageNumber(pageNumber int)
}

type SortInfo interface {
	GetSortInfo() []*SortPair
}

type SortAble struct {
	// format: "field[ desc], field2[ desc]"
	OrderBy string `json:"order_by" form:"order_by" auto_read:"order_by"`
}

type SortPair struct {
	Field        string
	IsDescending bool
}

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
