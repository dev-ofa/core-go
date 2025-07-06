package mongox

import (
	"context"
	"fmt"
	"github.com/dev-ofa/core-go/model/datax"
	"strconv"
	"testing"
	"time"

	"github.com/dev-ofa/core-go/model"

	"github.com/shiningrush/droplet/data"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type testEntity struct {
	model.Entity[string] `bson:"inline"`
	StrField             string `bson:"str_field"`
	IntField             int    `bson:"int_field"`

	model.UpdateAudit `bson:"inline"`
	model.TenantAudit `bson:"inline"`
}

type softTestEntity struct {
	model.Entity[string] `bson:"inline"`

	model.DeleteAudit `bson:"inline"`
}

var _ model.Repo[string, *testEntity] = (*CollectionLib[string, *testEntity])(nil)

func initTestMongoClient() (*mongo.Database, error) {
	cli, err := mongo.Connect(options.Client().ApplyURI("mongodb://root:example@10.37.39.64:27017/?connectTimeoutMS=2000"))
	if err != nil {
		return nil, fmt.Errorf("connect failed: %w", err)
	}

	return cli.Database("ofa-core-go"), nil
}

type CollectionLibTests struct {
	suite.Suite

	lib     *CollectionLib[string, *testEntity]
	softLib *CollectionLib[string, *softTestEntity]
	ctx     context.Context
}

func (ct *CollectionLibTests) SetupSuite() {
	cli, err := initTestMongoClient()
	if err != nil {
		panic(err)
	}

	ct.ctx = model.GenUserInfoContext(model.NewByteReqInfo())
	ct.lib = NewCollectionLib[string, *testEntity](cli.Collection("test_cls"))
	ct.softLib = NewCollectionLib[string, *softTestEntity](cli.Collection("soft_test_cls"))

	// ensure clean db
	ct.TearDownSuite()
	for i := 0; i < 8; i++ {
		_, err = ct.lib.Create(ct.ctx, &testEntity{
			Entity: model.Entity[string]{ID: strconv.Itoa(i)},
		})
		if err != nil {
			panic(err)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func (ct *CollectionLibTests) TearDownSuite() {
	if _, err := ct.lib.cls.DeleteMany(ct.ctx, bson.M{}); err != nil {
		panic(err)
	}
	if _, err := ct.softLib.cls.DeleteMany(ct.ctx, bson.M{}); err != nil {
		panic(err)
	}
}

func (ct *CollectionLibTests) cleanCollection() {
}

func (ct *CollectionLibTests) TestCreateUpdateAndGet() {
	lib := ct.lib
	cx := ct.ctx

	insertRet, err := lib.Create(cx, &testEntity{Entity: model.Entity[string]{ID: "10"}})
	ct.Require().NoError(err)
	getBody, err := lib.Get(cx, "10")
	ct.Require().NoError(err)
	err = lib.Delete(cx, insertRet)
	ct.Require().NoError(err)

	_, err = lib.Create(cx, &testEntity{})
	ct.Require().Error(err)

	// 只测试部分字段即可，核心在于验证DB访问
	ct.Equal("vinci", getBody.CreatedBy)
	ct.Equal("vinci", getBody.UpdatedBy)
	ct.Equal("tenant", getBody.TenantID)
	ct.Equal("app", getBody.AppID)

	// 开启修复选项(主从不一致场景较难模拟，保持正常功能即可)
	lib.opt.TryFixSyncDelay = model.FixedStrategyBackoff

	// Update
	getRet, err := lib.Get(cx, "1")
	ct.Require().NoError(err)
	// 验证不变的情况下不会误触发乐观锁
	_, err = lib.Update(cx, getRet)
	ct.Require().NoError(err)
	getRet.StrField = "new_str"
	getRet.IntField = 10
	_, err = lib.Update(cx, getRet)
	ct.Require().NoError(err)
	getBody, err = lib.Get(cx, "1")
	ct.Require().NoError(err)
	ct.Equal("new_str", getBody.StrField)
	ct.Equal(10, getBody.IntField)
	ct.NotEqual(getBody.CreatedAt, getBody.UpdatedAt)

	// Upsert
	insertRet, err = lib.Upsert(cx, &testEntity{Entity: model.Entity[string]{ID: "15"}})
	ct.Require().NoError(err)
	getBody, err = lib.Get(cx, "15")
	ct.Require().NoError(err)
	ct.ensureAuditField(getBody)

	getBody.StrField = "str"
	getBody.IntField = 100
	insertRet, err = lib.Upsert(cx, getBody)
	ct.Require().NoError(err)
	getBody, err = lib.Get(cx, "15")
	ct.Require().NoError(err)
	ct.NotEqual(getBody.CreatedAt, getBody.UpdatedAt)
	ct.ensureAuditField(getBody)
	ct.Equal("str", getBody.StrField)
	ct.Equal(100, getBody.IntField)
	err = lib.Delete(cx, getBody)
	ct.Require().NoError(err)

	// Patch
	err = lib.Patch(cx, &testEntity{Entity: model.Entity[string]{ID: "2"}, StrField: "patch_str"})
	ct.Require().NoError(err)
	getBody, err = lib.Get(cx, "2")
	ct.Require().NoError(err)
	ct.Equal("patch_str", getBody.StrField)
	ct.Equal(0, getBody.IntField)
	ct.NotEqual(getBody.CreatedAt, getBody.UpdatedAt)
}

func (ct *CollectionLibTests) ensureAuditField(entity *testEntity) {
	ct.Equal("vinci", entity.CreatedBy)
	ct.Equal("vinci", entity.UpdatedBy)
	ct.Equal("tenant", entity.TenantID)
	ct.Equal("app", entity.AppID)
}

func (ct *CollectionLibTests) TestSoftDelete() {
	lib := ct.softLib
	cx := ct.ctx
	se := &softTestEntity{Entity: model.Entity[string]{ID: "soft-test"}}
	// soft
	_, err := lib.Create(cx, se)
	ct.Require().NoError(err)
	getSe, err := lib.Get(cx, "soft-test")
	ct.Require().NoError(err)
	ct.Require().NotNil(getSe)
	err = lib.Delete(cx, getSe)
	ct.Require().NoError(err)
	getSe, err = lib.Get(cx, "soft-test")
	ct.Require().Equal(data.ErrNotFound, err)
	ct.Require().Nil(getSe)
	_, err = lib.Update(cx, se)
	ct.Require().Equal(data.ErrNotFound, err)

	// disable
	cx = model.SetCtxSoftDelete(cx, model.SoftDeleteDisable)
	getSe, err = lib.Get(cx, "soft-test")
	ct.Require().NoError(err)
	ct.Require().NotNil(getSe)
	_, err = lib.Update(cx, se)
	ct.Require().NoError(err)
	err = lib.Delete(cx, getSe)
	ct.Require().NoError(err)
	getSe, err = lib.Get(cx, "soft-test")
	ct.Require().Equal(data.ErrNotFound, err)
	ct.Require().Nil(getSe)

	// batch-delete
	cx = model.SetCtxSoftDelete(cx, model.SoftDeleteEnable)
	_, err = lib.Create(cx, se)
	ct.Require().NoError(err)
	_, err = lib.BatchDelete(cx, []*softTestEntity{se})
	ct.Require().NoError(err)
	_, err = lib.BatchDelete(cx, []*softTestEntity{se})
	ct.Require().Equal(data.ErrNotFound, err)
	cx = model.SetCtxSoftDelete(cx, model.SoftDeleteDisable)
	_, err = lib.BatchDelete(cx, []*softTestEntity{se})
	ct.Require().NoError(err)
	_, err = lib.BatchDelete(cx, []*softTestEntity{se})
	ct.Require().Equal(data.ErrNotFound, err)
}

type createError struct {
	model.Entity[string]
}
type updateError struct {
	model.Entity[string] `bson:"inline"`
	model.UpdateAudit
}
type updateMsError struct {
	model.Entity[string] `bson:"inline"`
	model.UpdateAuditMs
}
type deleteError struct {
	model.Entity[string] `bson:"inline"`
	model.DeleteAudit
}
type deleteMsError struct {
	model.Entity[string] `bson:"inline"`
	model.DeleteAuditMs
}
type tenantError struct {
	model.Entity[string] `bson:"inline"`
	model.TenantAudit
}
type ctxRecorderError struct {
	model.Entity[string] `bson:"inline"`
}

func (ct *CollectionLibTests) TestTypeCheck() {
	ct.PanicsWithValue("entity[createError] implement base[Entity[string]], should tag it with 'bson: inline'", func() {
		NewCollectionLib[string, *createError](ct.lib.cls)
	})
	ct.PanicsWithValue("entity[updateError] implement base[UpdateAudit], should tag it with 'bson: inline'", func() {
		NewCollectionLib[string, *updateError](ct.lib.cls)
	})
	ct.PanicsWithValue("entity[updateMsError] implement base[UpdateAuditMs], should tag it with 'bson: inline'", func() {
		NewCollectionLib[string, *updateMsError](ct.lib.cls)
	})
	ct.PanicsWithValue("entity[deleteError] implement base[DeleteAudit], should tag it with 'bson: inline'", func() {
		NewCollectionLib[string, *deleteError](ct.lib.cls)
	})
	ct.PanicsWithValue("entity[deleteMsError] implement base[DeleteAuditMs], should tag it with 'bson: inline'", func() {
		NewCollectionLib[string, *deleteMsError](ct.lib.cls)
	})
	// TODO: waiting implement
	//ct.PanicsWithValue("entity[tenantError] implement base[TenantAudit], should tag it with 'bson: inline'", func() {
	//	NewCollectionLib[string, *tenantError](ct.lib.cls)
	//})
}

func (ct *CollectionLibTests) TestDelete() {
	lib := ct.lib
	cx := ct.ctx
	cnt, err := lib.BatchDelete(cx, nil)
	ct.Require().NoError(err)
	ct.Zero(cnt)

	cnt, err = lib.BatchDeleteByIDs(cx, nil)
	ct.Require().NoError(err)
	ct.Zero(cnt)
}

func (ct *CollectionLibTests) TestQuery() {
	lib := ct.lib
	cx := ct.ctx
	ret, err := lib.Find(cx, bson.M{})
	ct.Require().NoError(err)
	ct.Equal(8, len(ret))
	ct.Equal("3", ret[3].ID)

	pageRet, err := lib.PageQuery(cx, &PageQueryInput{
		Pager: &model.Pager{PageSize: 2, PageNum: 2},
	})
	ct.Require().NoError(err)
	ct.Equal(2, len(pageRet.Rows))
	ct.Equal("2", pageRet.Rows[0].ID)
	ct.Equal(8, pageRet.TotalCount)

	// Page(no size)
	pageRet, err = lib.PageQuery(cx, &PageQueryInput{
		Pager: &model.Pager{},
	})
	ct.Equal(8, len(pageRet.Rows))
	ct.Equal("3", pageRet.Rows[3].ID)

	pageRet, err = lib.PageQuery(cx, &PageQueryInput{
		Pager: &model.Pager{PageSize: 2},
	})
	ct.Equal(2, len(pageRet.Rows))
	ct.Equal("1", pageRet.Rows[1].ID)

	// sort
	pageRet, err = lib.PageQuery(cx, &PageQueryInput{
		Pager: &model.Pager{},
		Sort:  &datax.SortAble{OrderBy: "created_at desc, updated_at desc"},
	})
	ct.Require().NoError(err)
	ct.Equal(8, len(pageRet.Rows))
	ct.Equal("4", pageRet.Rows[3].ID)
}

func (ct *CollectionLibTests) TestDataIsolation() {
	lib := ct.lib
	cx := ct.ctx

	reqInfo := model.NewByteReqInfo()
	reqInfo.UID = "bad-case"
	cx = model.GenUserInfoContext(reqInfo)
	cx = model.SetCtxRepoDataIsolation(cx, model.DataIsolationUser)
	ret, err := lib.Find(cx, bson.M{})
	ct.Require().NoError(err)
	ct.Require().Equal(0, len(ret))

	_, err = lib.Create(cx, &testEntity{Entity: model.Entity[string]{ID: "100"}})
	ct.Require().NoError(err)
	ret, err = lib.Find(cx, bson.M{})
	ct.Require().NoError(err)
	ct.Require().Equal(1, len(ret))

	// tenant
	reqInfo.TID = "bad-case"
	cx = model.GenUserInfoContext(reqInfo)
	cx = model.SetCtxRepoDataIsolation(cx, model.DataIsolationTenant)
	ret, err = lib.Find(cx, bson.M{})
	ct.Require().NoError(err)
	ct.Require().Equal(0, len(ret))

	_, err = lib.Create(cx, &testEntity{Entity: model.Entity[string]{ID: "101"}})
	ct.Require().NoError(err)
	ret, err = lib.Find(cx, bson.M{})
	ct.Require().NoError(err)
	ct.Require().Equal(1, len(ret))

	// app
	reqInfo.AID = "bad-case"
	cx = model.GenUserInfoContext(reqInfo)
	cx = model.SetCtxRepoDataIsolation(cx, model.DataIsolationApp)
	ret, err = lib.Find(cx, bson.M{})
	ct.Require().NoError(err)
	ct.Require().Equal(0, len(ret))

	_, err = lib.Create(cx, &testEntity{Entity: model.Entity[string]{ID: "102"}})
	ct.Require().NoError(err)
	ret, err = lib.Find(cx, bson.M{})
	ct.Require().NoError(err)
	ct.Require().Equal(1, len(ret))

	cx = model.SetCtxRepoDataIsolation(cx, model.DataIsolationNone)
	_, _ = lib.BatchDeleteByIDs(cx, []string{"100", "101", "102"})
}

// TODO: waiting implement
//func (ct *CollectionLibTests) TestDeployIsolation() {
//	lib := ct.lib
//	cx := ct.ctx
//
//	cx = model.SetCtxRepoDeployIsolation(cx, model.DeployIsolationCluster)
//	ret, err := lib.Find(cx, bson.M{})
//	ct.Require().NoError(err)
//	ct.Require().Equal(0, len(ret))
//
//	_, err = lib.Create(cx, &testEntity{Entity: model.Entity[string]{ID: "200"}})
//	ct.Require().NoError(err)
//	ret, err = lib.Find(cx, bson.M{})
//	ct.Require().NoError(err)
//	ct.Require().Equal(1, len(ret))
//	_, _ = lib.BatchDeleteByIDs(cx, []string{"200"})
//}

func TestMongoEx(t *testing.T) {
	suite.Run(t, new(CollectionLibTests))
}

func TestBuildPatchPayload(t *testing.T) {
	tests := []struct {
		caseDesc string
		givePtr  interface{}
		wantRet  bson.M
	}{
		{
			caseDesc: "normal",
			givePtr: struct {
				PubVal     string `bson:"pub_val"`
				OmitVal    string `bson:"omit_val,omitempty"`
				DefaultInt int
				Ignore     string `bson:"-"`
				ZeroInt    int
				ZeroBool   bool
				Embed      struct {
					PubVal    string `bson:"pub_val"`
					MoreEmbed struct {
						MoreField string `bson:"more_field"`
					} `bson:"more_embed"`
				} `bson:"embed"`
				InlineEmbed struct {
					InlineField string `bson:"inline_field"`
				} `bson:"inline"`
				privateVal string
			}{
				PubVal:     "pub",
				OmitVal:    "o",
				DefaultInt: 123,
				privateVal: "pri",
				Embed: struct {
					PubVal    string `bson:"pub_val"`
					MoreEmbed struct {
						MoreField string `bson:"more_field"`
					} `bson:"more_embed"`
				}{
					PubVal: "e_pub",
					MoreEmbed: struct {
						MoreField string `bson:"more_field"`
					}{
						MoreField: "more_values",
					},
				},
				InlineEmbed: struct {
					InlineField string `bson:"inline_field"`
				}{
					InlineField: "inline_val",
				},
			},
			wantRet: bson.M{
				"omit_val":                    "o",
				"defaultint":                  123,
				"pub_val":                     "pub",
				"embed.pub_val":               "e_pub",
				"embed.more_embed.more_field": "more_values",
				"inline_field":                "inline_val",
			},
		},
		{
			caseDesc: "ptr",
			givePtr: &struct {
				PubVal     string `bson:"pub_val"`
				DefaultInt int
				privateVal string
			}{
				PubVal:     "pub",
				DefaultInt: 123,
				privateVal: "pri",
			},
			wantRet: bson.M{
				"defaultint": 123,
				"pub_val":    "pub",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.caseDesc, func(t *testing.T) {
			ret := BuildPatchPayload(tc.givePtr)
			require.Equal(t, tc.wantRet, ret)
		})
	}
}
