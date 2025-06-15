package mongox

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/dev-ofa/core-go/model"
	"github.com/dev-ofa/core-go/pass"

	"github.com/avast/retry-go"
	"github.com/shiningrush/droplet/data"
	"github.com/shiningrush/goext/gtx"
	"github.com/shiningrush/goext/timex"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func BuildPatchPayload(ptr any) bson.M {
	return BuildPatchPayloadWithParent(reflect.ValueOf(ptr), "")
}

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

type CollectionLib[P model.IDType, T model.EntityConstraint[P]] struct {
	cls *mongo.Collection
	opt *model.RepoOpt

	idKey string
}

type PageQueryInput struct {
	Filter bson.M
	Pager  data.PagerInfo
	Sort   data.SortInfo
}

func (l *CollectionLib[P, T]) WithRepoOpt(opt *model.RepoOpt) *CollectionLib[P, T] {
	l.opt = opt
	return l
}

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
				return nil, fmt.Errorf("there is no user id in context")
			}
			filter["created_by"] = uid
		}
	case model.DataIsolationTenant:
		if _, ok := any(gtx.Zero[T]()).(model.TenantCarrier); ok {
			tid, ok := pass.CtxGetTenantID(ctx)
			if !ok {
				return nil, fmt.Errorf("there is no tenant id in context")
			}
			filter["tenant_id"] = tid
		}
	case model.DataIsolationApp:
		if _, ok := any(gtx.Zero[T]()).(model.TenantCarrier); ok {
			tid, ok := pass.CtxGetAppID(ctx)
			if !ok {
				return nil, fmt.Errorf("there is no app id in context")
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

func (l *CollectionLib[P, T]) Get(ctx context.Context, id P) (ret T, err error) {
	filter := bson.M{l.idKey: id}
	return l.GetByFilter(ctx, filter)
}

func (l *CollectionLib[P, T]) GetByFilter(ctx context.Context, filter bson.M) (ret T, err error) {
	filter, err = l.InjectCond(ctx, filter)
	if err != nil {
		return
	}

	opt := l.GetMergedRepoOpt(ctx)
	if opt.TryFixSyncDelay == model.FixedStrategyNone || opt.TryFixSyncDelay == "" {
		if err := l.cls.FindOne(ctx, filter).Decode(&ret); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return ret, data.ErrNotFound
			}

			return ret, fmt.Errorf("mongo find failed: %w", err)
		}

		return
	}

	// FixedStrategyBackoff 的情况下会针对 not found自动进行2次退避重试，尝试修正主从延迟带来的不一致性
	err = retry.Do(func() error {
		if err := l.cls.FindOne(ctx, filter).Decode(&ret); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return data.ErrNotFound
			}

			return fmt.Errorf("mongo find failed: %w", err)
		}
		return nil
	}, retry.Attempts(3),
		retry.LastErrorOnly(true),
		retry.Delay(time.Millisecond*50),
		retry.MaxJitter(time.Millisecond*50),
		retry.RetryIf(func(err error) bool {
			return data.ErrNotFound.Is(err)
		}))

	return
}

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
			return gtx.Zero[T](), data.ErrConflict
		}
		return gtx.Zero[T](), fmt.Errorf("create doc failed: %w", err)
	}

	return doc, nil
}

func (l *CollectionLib[P, T]) checkIfIDExisted(doc T) error {
	if gtx.IsZero(doc.GetID()) {
		return fmt.Errorf("id can not be empty")
	}
	return nil
}

func (l *CollectionLib[P, T]) Update(ctx context.Context, doc T) (ret T, err error) {
	return l.commonReplace(ctx, doc, false)
}

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
			return gtx.Zero[T](), data.ErrConflict
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
			return gtx.Zero[T](), data.NewConflictError("data is modified by other")
		}

		return gtx.Zero[T](), data.ErrNotFound
	}

	if _, ok := filter["updated_at"]; ok && rpRet.ModifiedCount == 0 {
		return gtx.Zero[T](), data.NewConflictError("optimistic locking failed")
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

type PatchRawInput struct {
	Filter         bson.M
	PatchPayload   bson.M
	IsMany         bool
	SkipInjectCond bool
}

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
			return fmt.Errorf("there is no user in context")
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
		return data.ErrNotFound
	}

	if _, ok := input.Filter["updated_at"]; ok && ret.ModifiedCount == 0 {
		return data.NewConflictError("optimistic locking failed")
	}

	return nil
}

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
		return data.ErrNotFound
	}

	return
}

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
			return data.ErrConflict
		}
		return fmt.Errorf("batch create doc failed: %w", err)
	}

	return nil
}

func (l *CollectionLib[P, T]) BatchUpdate(ctx context.Context, docs []T) error {
	for _, v := range docs {
		if _, err := l.Update(ctx, v); err != nil {
			return err
		}
	}
	return nil
}

func (l *CollectionLib[P, T]) BatchDelete(ctx context.Context, docs []T) (int, error) {
	var ids []P
	for _, v := range docs {
		ids = append(ids, v.GetID())
	}

	return l.BatchDeleteByIDs(ctx, ids)
}

func (l *CollectionLib[P, T]) BatchDeleteByIDs(ctx context.Context, ids []P) (cnt int, err error) {
	if len(ids) == 0 {
		return 0, nil
	}

	filter := bson.M{l.idKey: bson.M{"$in": ids}}
	return l.BatchDeleteByFilter(ctx, filter)
}

func (l *CollectionLib[P, T]) BatchDeleteByFilter(ctx context.Context, filter bson.M) (cnt int, err error) {
	filter, err = l.InjectCond(ctx, filter)
	if err != nil {
		return
	}

	opt := l.GetMergedRepoOpt(ctx)
	if _, ok := any(gtx.Zero[T]()).(model.DeleteAuditor); ok && opt.SoftDelete != model.SoftDeleteDisable {

		u, ok := pass.CtxGetOperator(ctx)
		if !ok {
			return 0, fmt.Errorf("there is no user")
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
			return 0, data.ErrNotFound
		}

		return int(ret.MatchedCount), nil
	}

	ret, err := l.cls.DeleteMany(ctx, filter)
	if err != nil {
		return 0, fmt.Errorf("batch delete mongo failed: %w", err)
	}

	if ret.DeletedCount == 0 {
		return 0, data.ErrNotFound
	}

	return int(ret.DeletedCount), nil
}

func Ptr[T any](val T) *T {
	return &val
}
