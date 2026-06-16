package mongox

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/dev-ofa/core-go/model/datax"

	"github.com/dev-ofa/core-go/model"
	"github.com/dev-ofa/core-go/pass"

	"github.com/avast/retry-go"
	"github.com/shiningrush/goext/gtx"
	"github.com/shiningrush/goext/timex"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// BuildPatchPayload builds a bson patch payload from a struct pointer.
func BuildPatchPayload(ptr any) bson.M {
	return BuildPatchPayloadWithParent(reflect.ValueOf(ptr), "")
}

// BuildPatchPayloadWithParent builds a patch payload with parent key prefix.
func BuildPatchPayloadWithParent(v reflect.Value, parent string) bson.M {
	t := v.Type()
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
		v = v.Elem()
	}

	if t.Kind() != reflect.Struct {
		return bson.M{}
	}

	payload := bson.M{}
	for i := 0; i < t.NumField(); i++ {
		if !t.Field(i).IsExported() {
			continue
		}

		if v.Field(i).IsZero() {
			continue
		}

		name := strings.ToLower(t.Field(i).Name)
		if mongoTag, ok := t.Field(i).Tag.Lookup("bson"); ok {
			name = mongoTag
		}
		name = strings.Split(name, ",")[0]
		if name == "-" {
			continue
		}
		if parent != "inline" {
			name = genDbKey(parent, name)
		}

		if v.Field(i).Kind() == reflect.Struct {
			for k, v := range BuildPatchPayloadWithParent(v.Field(i), name) {
				payload[k] = v
			}
			continue
		}

		payload[name] = v.Field(i).Interface()
	}

	return payload
}

func genDbKey(parent, fieldKey string) string {
	if parent == "" {
		return fieldKey
	}
	return fmt.Sprintf("%s.%s", parent, fieldKey)
}

// NewCollectionLib creates a Mongo collection helper for a given entity type.
func NewCollectionLib[P model.IDType, T model.EntityConstraint[P]](cls *mongo.Collection) *CollectionLib[P, T] {
	if cls == nil {
		panic("cls should not be empty")
	}

	checks := []any{
		model.Entity[P]{},
		model.CreateAudit{},
		model.CreateAuditMs{},
		model.UpdateAudit{},
		model.UpdateAuditMs{},
		model.DeleteAudit{},
		model.DeleteAuditMs{},
		model.TenantAudit{},
	}
	typeOfEntity := reflect.TypeOf(gtx.Zero[T]())
	if typeOfEntity.Kind() == reflect.Ptr {
		typeOfEntity = typeOfEntity.Elem()
	}
	for i := 0; i < typeOfEntity.NumField(); i++ {
		field := typeOfEntity.Field(i)
		for _, v := range checks {
			if field.Type.ConvertibleTo(reflect.TypeOf(v)) {
				if bsonTag := field.Tag.Get("bson"); !strings.Contains(bsonTag, "inline") {
					panic(fmt.Sprintf("entity[%s] implement base[%s], should tag it with 'bson: inline'",
						typeOfEntity.Name(), reflect.TypeOf(v).Name()))
				}
			}
		}
	}

	return &CollectionLib[P, T]{
		cls:   cls,
		idKey: "_id",
		opt:   &model.RepoOpt{},
	}
}

// CollectionLib is a Mongo collection helper with repo options.
type CollectionLib[P model.IDType, T model.EntityConstraint[P]] struct {
	cls *mongo.Collection
	opt *model.RepoOpt

	idKey string
}

// PageQueryInput wraps filter, pager, and sorter for paging queries.
type PageQueryInput struct {
	// Filter is the Mongo filter.
	Filter bson.M
	// Pager is the paging input.
	Pager datax.PagerInfo
	// Sort is the sorting input.
	Sort datax.SortInfo
}

// FeedQueryInput wraps filter, pager, and cursor field for feed queries.
type FeedQueryInput struct {
	// Filter is the Mongo filter.
	Filter bson.M
	// Pager is the feed paging input. PageSize is used; PageNum is ignored.
	Pager datax.PagerInfo
	// CursorField is the single field used as feed cursor. Defaults to _id.
	CursorField string
	// IsDescending controls cursor sort direction.
	IsDescending bool
}

// WithRepoOpt sets repo options on the collection helper.
func (l *CollectionLib[P, T]) WithRepoOpt(opt *model.RepoOpt) *CollectionLib[P, T] {
	l.opt = opt
	return l
}

// GetMergedRepoOpt merges context repo options with local options.
func (l *CollectionLib[P, T]) GetMergedRepoOpt(ctx context.Context) *model.RepoOpt {
	return model.CtxMergeRepoOpt(ctx, l.opt)
}

func (l *CollectionLib[P, T]) injectIsolationCond(ctx context.Context, filter bson.M) (bson.M, error) {
	opt := l.GetMergedRepoOpt(ctx)

	switch opt.DataIsolation {
	case model.DataIsolationUser:
		if _, ok := any(gtx.Zero[T]()).(model.CreateAuditor); ok {
			uid, ok := pass.CtxGetOperator(ctx)
			if !ok {
				return nil, datax.NewValidationError("there is no user id in context", nil, nil)
			}
			filter["created_by"] = uid
		}
	case model.DataIsolationTenant:
		if _, ok := any(gtx.Zero[T]()).(model.TenantCarrier); ok {
			tid, ok := pass.CtxGetTenantID(ctx)
			if !ok {
				return nil, datax.NewValidationError("there is no tenant id in context", nil, nil)
			}
			filter["tenant_id"] = tid
		}
	case model.DataIsolationApp:
		if _, ok := any(gtx.Zero[T]()).(model.TenantCarrier); ok {
			tid, ok := pass.CtxGetAppID(ctx)
			if !ok {
				return nil, datax.NewValidationError("there is no app id in context", nil, nil)
			}
			filter["app_id"] = tid
		}
	}

	// TODO: deploy isolation
	//if _, ok := any(gtx.Zero[T]()).(runenv.Recorder); ok {
	//	switch opt.DeployIsolation {
	//	case model.DeployIsolationCluster:
	//		filter["run_context.deploy.cluster"] = env.Cluster()
	//	case model.DeployIsolationEnv:
	//		filter["run_context.deploy.env"] = runenv.GetEnv()
	//	}
	//}

	return filter, nil
}

func (l *CollectionLib[P, T]) injectSoftDeleteCond(ctx context.Context, filter bson.M) (bson.M, error) {
	opt := l.GetMergedRepoOpt(ctx)
	if opt.SoftDelete == model.SoftDeleteDisable {
		return filter, nil
	}

	if _, ok := any(gtx.Zero[T]()).(model.DeleteAuditor); ok {
		filter["deleted_at"] = bson.M{"$exists": false}
	}
	return filter, nil
}

// InjectCond injects isolation and soft-delete conditions into filter.
func (l *CollectionLib[P, T]) InjectCond(ctx context.Context, filter bson.M) (bson.M, error) {
	if filter == nil {
		filter = bson.M{}
	}
	filter, err := l.injectIsolationCond(ctx, filter)
	if err != nil {
		return nil, err
	}

	filter, err = l.injectSoftDeleteCond(ctx, filter)
	if err != nil {
		return nil, err
	}
	return filter, nil
}

// Find queries documents by filter.
func (l *CollectionLib[P, T]) Find(ctx context.Context, filter bson.M, opts ...options.Lister[options.FindOptions]) (ret []T, err error) {
	filter, err = l.InjectCond(ctx, filter)
	if err != nil {
		return nil, err
	}

	cur, err := l.cls.Find(ctx, filter, opts...)
	if err != nil {
		return nil, fmt.Errorf("find mongo failed: %w", err)
	}

	if err := cur.All(ctx, &ret); err != nil {
		return nil, fmt.Errorf("cursor decode failed: %w", err)
	}

	return
}

// PageQuery performs a paged query with filter, sort, and paging input.
func (l *CollectionLib[P, T]) PageQuery(ctx context.Context, input *PageQueryInput) (ret *model.PagedResult[T], err error) {
	input.Filter, err = l.InjectCond(ctx, input.Filter)
	if err != nil {
		return nil, err
	}

	count, err := l.cls.CountDocuments(ctx, input.Filter)
	if err != nil {
		return nil, fmt.Errorf("count document failed: %w", err)
	}

	ret = &model.PagedResult[T]{}
	ret.TotalCount = int(count)

	opt := options.Find()
	l.modifyPageOpt(opt, input)
	l.modifySortOpt(opt, input)

	cur, err := l.cls.Find(ctx, input.Filter, opt)
	if err != nil {
		return nil, fmt.Errorf("find result failed: %w", err)
	}
	if err := cur.All(ctx, &ret.Rows); err != nil {
		return nil, fmt.Errorf("unmarshal result failed: %w", err)
	}

	return ret, nil
}

// FeedQuery performs a single-field cursor-based feed query without counting total rows.
func (l *CollectionLib[P, T]) FeedQuery(ctx context.Context, input *FeedQueryInput) (ret *model.FeedResult[T], err error) {
	pageSize, _, pageToken := 0, 0, ""
	if input.Pager != nil {
		pageSize, _, pageToken = input.Pager.GetPageInfo()
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	cursorField := l.normalizeFeedCursorField(input.CursorField)

	input.Filter, err = l.InjectCond(ctx, input.Filter)
	if err != nil {
		return nil, err
	}
	if pageToken != "" {
		cursorFilter, err := l.buildFeedCursorFilter(cursorField, pageToken, input.IsDescending)
		if err != nil {
			return nil, fmt.Errorf("build feed cursor filter failed: %w", err)
		}
		input.Filter = mergeFeedFilter(input.Filter, cursorFilter)
	}

	opt := options.Find()
	opt.SetLimit(int64(pageSize + 1))
	opt.SetSort(bson.D{{Key: cursorField, Value: feedSortOrder(input.IsDescending)}})

	cur, err := l.cls.Find(ctx, input.Filter, opt)
	if err != nil {
		return nil, fmt.Errorf("find feed result failed: %w", err)
	}

	rows := make([]T, 0, pageSize+1)
	if err := cur.All(ctx, &rows); err != nil {
		return nil, fmt.Errorf("unmarshal feed result failed: %w", err)
	}

	ret = &model.FeedResult[T]{
		Rows: rows,
	}
	if len(rows) > pageSize {
		ret.Rows = rows[:pageSize]
		ret.NextPageToken, err = l.feedCursorToken(ret.Rows[len(ret.Rows)-1], cursorField)
		if err != nil {
			return nil, fmt.Errorf("build next page token failed: %w", err)
		}
	}

	return ret, nil
}

func (l *CollectionLib[P, T]) normalizeFeedCursorField(cursorField string) string {
	if cursorField == "" || cursorField == "id" {
		return l.idKey
	}
	return cursorField
}

func (l *CollectionLib[P, T]) buildFeedCursorFilter(cursorField string, pageToken string, isDescending bool) (bson.M, error) {
	op := "$gt"
	if isDescending {
		op = "$lt"
	}
	if cursorField == l.idKey {
		var zero P
		value, err := parseFeedTokenByType(pageToken, reflect.TypeOf(zero))
		if err != nil {
			return nil, err
		}
		return bson.M{cursorField: bson.M{op: value}}, nil
	}
	value, err := l.parseFeedCursorToken(cursorField, pageToken)
	if err != nil {
		return nil, err
	}
	return bson.M{cursorField: bson.M{op: value}}, nil
}

func feedSortOrder(isDescending bool) int {
	if isDescending {
		return -1
	}
	return 1
}

func (l *CollectionLib[P, T]) parseFeedCursorToken(cursorField string, pageToken string) (any, error) {
	fieldType, ok := findBSONFieldType(reflect.TypeOf(gtx.Zero[T]()), cursorField)
	if !ok {
		return nil, datax.NewValidationError(fmt.Sprintf("cursor field %s not found", cursorField), nil, nil)
	}
	return parseFeedTokenByType(pageToken, fieldType)
}

func parseFeedTokenByType(pageToken string, fieldType reflect.Type) (any, error) {
	for fieldType.Kind() == reflect.Ptr {
		fieldType = fieldType.Elem()
	}
	if fieldType == reflect.TypeOf(time.Time{}) {
		v, err := time.Parse(time.RFC3339Nano, pageToken)
		if err != nil {
			return nil, datax.NewValidationError("parse time feed token failed", nil, err)
		}
		return v, nil
	}
	switch fieldType.Kind() {
	case reflect.String:
		return reflect.ValueOf(pageToken).Convert(fieldType).Interface(), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v, err := strconv.ParseInt(pageToken, 10, fieldType.Bits())
		if err != nil {
			return nil, datax.NewValidationError("parse int feed token failed", nil, err)
		}
		return reflect.ValueOf(v).Convert(fieldType).Interface(), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v, err := strconv.ParseUint(pageToken, 10, fieldType.Bits())
		if err != nil {
			return nil, datax.NewValidationError("parse uint feed token failed", nil, err)
		}
		return reflect.ValueOf(v).Convert(fieldType).Interface(), nil
	case reflect.Float32, reflect.Float64:
		v, err := strconv.ParseFloat(pageToken, fieldType.Bits())
		if err != nil {
			return nil, datax.NewValidationError("parse float feed token failed", nil, err)
		}
		return reflect.ValueOf(v).Convert(fieldType).Interface(), nil
	default:
		return nil, datax.NewValidationError(fmt.Sprintf("unsupported cursor field type %s", fieldType.String()), nil, nil)
	}
}

func (l *CollectionLib[P, T]) feedCursorToken(row T, cursorField string) (string, error) {
	if cursorField == l.idKey {
		return fmt.Sprint(row.GetID()), nil
	}

	value, ok := findBSONFieldValue(reflect.ValueOf(row), cursorField)
	if !ok {
		return "", datax.NewValidationError(fmt.Sprintf("cursor field %s not found", cursorField), nil, nil)
	}
	if t, ok := value.(time.Time); ok {
		return t.Format(time.RFC3339Nano), nil
	}
	return fmt.Sprint(value), nil
}

func findBSONFieldType(entityType reflect.Type, cursorField string) (reflect.Type, bool) {
	for entityType.Kind() == reflect.Ptr {
		entityType = entityType.Elem()
	}
	if entityType.Kind() != reflect.Struct {
		return nil, false
	}
	for i := 0; i < entityType.NumField(); i++ {
		field := entityType.Field(i)
		if !field.IsExported() {
			continue
		}
		name, inline, skip := parseBSONFieldName(field)
		if skip {
			continue
		}
		if inline {
			if fieldType, ok := findBSONFieldType(field.Type, cursorField); ok {
				return fieldType, true
			}
			continue
		}
		if name == cursorField {
			return field.Type, true
		}
	}
	return nil, false
}

func findBSONFieldValue(value reflect.Value, cursorField string) (any, bool) {
	for value.Kind() == reflect.Ptr {
		if value.IsNil() {
			return nil, false
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return nil, false
	}
	valueType := value.Type()
	for i := 0; i < valueType.NumField(); i++ {
		field := valueType.Field(i)
		if !field.IsExported() {
			continue
		}
		name, inline, skip := parseBSONFieldName(field)
		if skip {
			continue
		}
		fieldValue := value.Field(i)
		if inline {
			if v, ok := findBSONFieldValue(fieldValue, cursorField); ok {
				return v, true
			}
			continue
		}
		if name == cursorField {
			return fieldValue.Interface(), true
		}
	}
	return nil, false
}

func parseBSONFieldName(field reflect.StructField) (name string, inline bool, skip bool) {
	name = strings.ToLower(field.Name)
	if mongoTag, ok := field.Tag.Lookup("bson"); ok {
		name = mongoTag
	}
	parts := strings.Split(name, ",")
	name = parts[0]
	if name == "-" {
		return "", false, true
	}
	for _, part := range parts[1:] {
		if part == "inline" {
			inline = true
			break
		}
	}
	if name == "inline" {
		inline = true
	}
	return name, inline, false
}

func mergeFeedFilter(base bson.M, cursor bson.M) bson.M {
	if len(base) == 0 {
		return cursor
	}
	if len(cursor) == 0 {
		return base
	}
	return bson.M{"$and": bson.A{base, cursor}}
}

func (l *CollectionLib[P, T]) modifyPageOpt(opt *options.FindOptionsBuilder, input *PageQueryInput) {
	pSize, pNum := 0, 0
	if input.Pager != nil {
		pSize, pNum, _ = input.Pager.GetPageInfo()
	}
	if pSize > 0 {
		opt.SetLimit(int64(pSize))
		if pNum > 0 {
			opt.SetSkip(int64(pSize * (pNum - 1)))
		}
	}
}

func (l *CollectionLib[P, T]) modifySortOpt(opt *options.FindOptionsBuilder, input *PageQueryInput) {
	sortConf := bson.D{}
	if input.Sort != nil {
		sortPairs := input.Sort.GetSortInfo()
		for _, v := range sortPairs {
			order := 1
			if v.IsDescending {
				order = -1
			}
			sortConf = append(sortConf, bson.E{Key: v.Field, Value: order})
		}
	}

	if len(sortConf) == 0 {
		if _, ok := any(gtx.Zero[T]()).(model.CreateAuditor); ok {
			sortConf = append(sortConf, bson.E{Key: "created_at", Value: 1})
		}
	}

	opt.SetSort(sortConf)
}

// Get fetches one document by id.
func (l *CollectionLib[P, T]) Get(ctx context.Context, id P) (ret T, err error) {
	filter := bson.M{l.idKey: id}
	return l.GetByFilter(ctx, filter)
}

// GetByFilter fetches one document by filter with optional retry strategy.
func (l *CollectionLib[P, T]) GetByFilter(ctx context.Context, filter bson.M) (ret T, err error) {
	filter, err = l.InjectCond(ctx, filter)
	if err != nil {
		return
	}
	resource := l.resourceByFilter(filter)

	opt := l.GetMergedRepoOpt(ctx)
	if opt.TryFixSyncDelay == model.FixedStrategyNone || opt.TryFixSyncDelay == "" {
		if err := l.cls.FindOne(ctx, filter).Decode(&ret); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return ret, datax.NewResourceNotFoundError(resource, nil)
			}

			return ret, fmt.Errorf("mongo find failed: %w", err)
		}

		return
	}

	// FixedStrategyBackoff 的情况下会针对 not found自动进行2次退避重试，尝试修正主从延迟带来的不一致性
	err = retry.Do(func() error {
		if err := l.cls.FindOne(ctx, filter).Decode(&ret); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return datax.NewResourceNotFoundError(resource, nil)
			}

			return fmt.Errorf("mongo find failed: %w", err)
		}
		return nil
	}, retry.Attempts(3),
		retry.LastErrorOnly(true),
		retry.Delay(time.Millisecond*50),
		retry.MaxJitter(time.Millisecond*50),
		retry.RetryIf(func(err error) bool {
			return datax.IsErrCode(datax.ErrCodeNotFound, err)
		}))

	return
}

// Create inserts a new document with audit fields applied.
func (l *CollectionLib[P, T]) Create(ctx context.Context, doc T) (T, error) {
	if err := l.checkIfIDExisted(doc); err != nil {
		return gtx.Zero[T](), err
	}

	if err := model.CtxCreateAudit(ctx, doc); err != nil {
		return gtx.Zero[T](), fmt.Errorf("audit doc failed: %w", err)
	}

	_, err := l.cls.InsertOne(ctx, doc)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return gtx.Zero[T](), datax.NewResourceConflictError(l.resourceByDoc(doc), nil)
		}
		return gtx.Zero[T](), fmt.Errorf("create doc failed: %w", err)
	}

	return doc, nil
}

func (l *CollectionLib[P, T]) checkIfIDExisted(doc T) error {
	if gtx.IsZero(doc.GetID()) {
		return datax.NewValidationError("id can not be empty", nil, nil)
	}
	return nil
}

// Update replaces a document, using optimistic checks when configured.
func (l *CollectionLib[P, T]) Update(ctx context.Context, doc T) (ret T, err error) {
	return l.commonReplace(ctx, doc, false)
}

// Upsert replaces or inserts a document.
func (l *CollectionLib[P, T]) Upsert(ctx context.Context, doc T) (ret T, err error) {
	return l.commonReplace(ctx, doc, true)
}

func (l *CollectionLib[P, T]) commonReplace(ctx context.Context, doc T, isUpsert bool) (ret T, err error) {
	filter, err := l.auditAndBuildReplaceFilter(ctx, doc, isUpsert)
	if err != nil {
		return gtx.Zero[T](), err
	}

	rpRet, err := l.cls.ReplaceOne(ctx, filter, doc, options.Replace().SetUpsert(isUpsert))
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return gtx.Zero[T](), datax.NewResourceConflictError(l.resourceByDoc(doc), nil)
		}
		return gtx.Zero[T](), fmt.Errorf("create doc failed: %w", err)
	}

	if isUpsert && rpRet.UpsertedCount > 0 {
		return doc, nil
	}

	if rpRet.MatchedCount == 0 {
		delete(filter, "updated_at")
		cnt, err := l.cls.CountDocuments(ctx, filter)
		if err != nil {
			return gtx.Zero[T](), fmt.Errorf("check id failed: %w", err)
		}
		if cnt > 0 {
			return gtx.Zero[T](), datax.NewResourceError(datax.ErrCodeConflict, "data is modified by other", l.resourceByFilter(filter), nil)
		}

		return gtx.Zero[T](), datax.NewResourceNotFoundError(l.resourceByFilter(filter), nil)
	}

	if _, ok := filter["updated_at"]; ok && rpRet.ModifiedCount == 0 {
		return gtx.Zero[T](), datax.NewResourceError(datax.ErrCodeConflict, "optimistic locking failed", l.resourceByFilter(filter), nil)
	}

	return doc, nil
}

func (l *CollectionLib[P, T]) auditAndBuildReplaceFilter(ctx context.Context, doc T, isUpsert bool) (bson.M, error) {
	if !isUpsert {
		return l.baseUpdateOp(ctx, doc)
	}

	cr, ok := any(doc).(model.CreateAuditor)
	if !ok {
		return l.baseUpdateOp(ctx, doc)
	}

	if _, createTime := cr.GetCreatorInfo(); createTime.IsZero() {
		filter := bson.M{
			l.idKey: doc.GetID(),
		}
		if err := model.CtxCreateAudit(ctx, doc); err != nil {
			return nil, err
		}
		return filter, nil
	}
	return l.baseUpdateOp(ctx, doc)
}

func (l *CollectionLib[P, T]) baseUpdateOp(ctx context.Context, doc T) (bson.M, error) {
	ret, err := model.UpdateLockAndAudit(ctx, doc, l.GetMergedRepoOpt(ctx))
	if err != nil {
		return nil, err
	}

	filter := bson.M{
		l.idKey: doc.GetID(),
	}
	if ret.HasOriginalUpdate {
		filter["updated_at"] = ret.OriginalUpdatedAt
	}
	filter, err = l.InjectCond(ctx, filter)
	if err != nil {
		return nil, err
	}

	return filter, nil
}

// Patch updates non-zero fields on a document by id.
func (l *CollectionLib[P, T]) Patch(ctx context.Context, doc T) (err error) {
	filter, err := l.baseUpdateOp(ctx, doc)
	if err != nil {
		return err
	}

	patchPayload := BuildPatchPayload(doc)
	return l.PatchRaw(ctx, &PatchRawInput{
		Filter:         filter,
		PatchPayload:   patchPayload,
		IsMany:         false,
		SkipInjectCond: true,
	})
}

// PatchRawInput defines raw patch parameters.
type PatchRawInput struct {
	// Filter is the Mongo filter for patch.
	Filter bson.M
	// PatchPayload is the update payload.
	PatchPayload bson.M
	// IsMany controls UpdateMany vs UpdateOne.
	IsMany bool
	// SkipInjectCond skips isolation and soft delete injection.
	SkipInjectCond bool
}

// PatchRaw applies a patch payload with optional filter injection.
func (l *CollectionLib[P, T]) PatchRaw(ctx context.Context, input *PatchRawInput) (err error) {
	if !input.SkipInjectCond {
		input.Filter, err = l.InjectCond(ctx, input.Filter)
		if err != nil {
			return err
		}
	}

	if _, ok := any(gtx.Zero[T]()).(model.UpdateAuditor); ok {
		u, hasUser := pass.CtxGetOperator(ctx)
		if !hasUser {
			return datax.NewValidationError("there is no user in context", nil, nil)
		}
		input.PatchPayload["updated_at"] = timex.Now()
		input.PatchPayload["updated_by"] = u
	}

	var ret *mongo.UpdateResult
	if input.IsMany {
		ret, err = l.cls.UpdateMany(ctx, input.Filter, bson.M{"$set": input.PatchPayload})
	} else {
		ret, err = l.cls.UpdateOne(ctx, input.Filter, bson.M{"$set": input.PatchPayload})
	}

	if err != nil {
		return fmt.Errorf("patch doc failed: %w", err)
	}

	if ret.MatchedCount == 0 {
		return datax.NewResourceNotFoundError(l.resourceByFilter(input.Filter), nil)
	}

	if _, ok := input.Filter["updated_at"]; ok && ret.ModifiedCount == 0 {
		return datax.NewResourceError(datax.ErrCodeConflict, "optimistic locking failed", l.resourceByFilter(input.Filter), nil)
	}

	return nil
}

// Delete deletes a document or applies soft delete when enabled.
func (l *CollectionLib[P, T]) Delete(ctx context.Context, doc T) (err error) {
	hasDeleteAudit, err := model.CtxDeleteAudit(ctx, doc)
	if err != nil {
		return fmt.Errorf("audit doc failed: %w", err)
	}

	opt := l.GetMergedRepoOpt(ctx)
	if hasDeleteAudit && opt.SoftDelete != model.SoftDeleteDisable {
		if _, err := l.Update(ctx, doc); err != nil {
			return err
		}
		return nil
	}

	filter, err := l.InjectCond(ctx, bson.M{l.idKey: doc.GetID()})
	if err != nil {
		return
	}
	dr, err := l.cls.DeleteOne(ctx, filter)
	if err != nil {
		return err
	}
	if dr.DeletedCount == 0 {
		return datax.NewResourceNotFoundError(l.resourceByDoc(doc), nil)
	}

	return
}

// BatchCreate inserts documents in batch without transactional guarantees.
func (l *CollectionLib[P, T]) BatchCreate(ctx context.Context, docs []T) error {
	var mongoDocs []interface{}
	for _, doc := range docs {
		if err := l.checkIfIDExisted(doc); err != nil {
			return err
		}

		if err := model.CtxCreateAudit(ctx, doc); err != nil {
			return fmt.Errorf("audit doc[%+v] failed: %w", doc, err)
		}
		mongoDocs = append(mongoDocs, doc)
	}

	_, err := l.cls.InsertMany(ctx, mongoDocs)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return datax.NewResourceConflictError(l.resourceByDocs(docs), nil)
		}
		return fmt.Errorf("batch create doc failed: %w", err)
	}

	return nil
}

// BatchUpdate updates documents one by one.
func (l *CollectionLib[P, T]) BatchUpdate(ctx context.Context, docs []T) error {
	for _, v := range docs {
		if _, err := l.Update(ctx, v); err != nil {
			return err
		}
	}
	return nil
}

// BatchDelete deletes documents by ids.
func (l *CollectionLib[P, T]) BatchDelete(ctx context.Context, docs []T) (int, error) {
	var ids []P
	for _, v := range docs {
		ids = append(ids, v.GetID())
	}

	return l.BatchDeleteByIDs(ctx, ids)
}

// BatchDeleteByIDs deletes documents by id list.
func (l *CollectionLib[P, T]) BatchDeleteByIDs(ctx context.Context, ids []P) (cnt int, err error) {
	if len(ids) == 0 {
		return 0, nil
	}

	filter := bson.M{l.idKey: bson.M{"$in": ids}}
	return l.BatchDeleteByFilter(ctx, filter)
}

// BatchDeleteByFilter deletes documents by filter with isolation rules.
func (l *CollectionLib[P, T]) BatchDeleteByFilter(ctx context.Context, filter bson.M) (cnt int, err error) {
	filter, err = l.InjectCond(ctx, filter)
	if err != nil {
		return
	}

	opt := l.GetMergedRepoOpt(ctx)
	if _, ok := any(gtx.Zero[T]()).(model.DeleteAuditor); ok && opt.SoftDelete != model.SoftDeleteDisable {

		u, ok := pass.CtxGetOperator(ctx)
		if !ok {
			return 0, datax.NewValidationError("there is no user", nil, nil)
		}

		updatePayload := &bson.M{
			"$set": bson.M{
				"deleted_at": timex.Now(),
				"deleted_by": u,
			},
		}
		ret, err := l.cls.UpdateMany(ctx, filter, updatePayload)
		if err != nil {
			return 0, fmt.Errorf("batch delete mongo failed: %w", err)
		}

		if ret.MatchedCount == 0 {
			return 0, datax.NewResourceNotFoundError(l.resourceByFilter(filter), nil)
		}

		return int(ret.MatchedCount), nil
	}

	ret, err := l.cls.DeleteMany(ctx, filter)
	if err != nil {
		return 0, fmt.Errorf("batch delete mongo failed: %w", err)
	}

	if ret.DeletedCount == 0 {
		return 0, datax.NewResourceNotFoundError(l.resourceByFilter(filter), nil)
	}

	return int(ret.DeletedCount), nil
}

func (l *CollectionLib[P, T]) resourceByID(id any) string {
	return fmt.Sprintf("%s/%v", l.cls.Name(), id)
}

func (l *CollectionLib[P, T]) resourceByDoc(doc T) string {
	return l.resourceByID(doc.GetID())
}

func (l *CollectionLib[P, T]) resourceByDocs(docs []T) string {
	ids := make([]P, 0, len(docs))
	for _, doc := range docs {
		ids = append(ids, doc.GetID())
	}
	return fmt.Sprintf("%s ids=%v", l.cls.Name(), ids)
}

func (l *CollectionLib[P, T]) resourceByFilter(filter bson.M) string {
	if id, ok := filter[l.idKey]; ok {
		return l.resourceByID(id)
	}
	return fmt.Sprintf("%s filter=%v", l.cls.Name(), filter)
}

// Ptr returns a pointer to val.
func Ptr[T any](val T) *T {
	return &val
}
