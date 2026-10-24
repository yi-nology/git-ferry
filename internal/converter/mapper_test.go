package converter

import (
	"reflect"
	"testing"
	"time"

	"github.com/yi-nology/git-ferry-core/model"
	olmodel "github.com/yi-nology/git-ferry/biz/model/operation_log"
	platformmodel "github.com/yi-nology/git-ferry/biz/model/platform"
	repomodel "github.com/yi-nology/git-ferry/biz/model/repo"
	taskmodel "github.com/yi-nology/git-ferry/biz/model/sync_task"
	webhookmodel "github.com/yi-nology/git-ferry/biz/model/webhook"
)

// ===== 机制本身 =====

// TestNormalizeField 归一化规则:只去 '_' 并转小写。
//
// core 与 thrift 的缩写差异(URL/Url、LFS/Lfs、SHA/Sha、TLS/Tls)恰好只差大小写,
// 归一化后就能对上——这是"core 加字段时转换层不用改"的前提。
func TestNormalizeField(t *testing.T) {
	cases := [][2]string{
		{"InstanceURL", "InstanceUrl"},
		{"GitLFS", "GitLfs"},
		{"CommitSHA", "CommitSha"},
		{"EventID", "EventId"},
		{"WebhookEventID", "WebhookEventId"},
		{"APIURL", "ApiUrl"},
		{"CACertPath", "CaCertPath"},
		{"SkipTLSVerify", "SkipTlsVerify"},
		{"SourceRepoKey", "SourceRepoKey"},
	}
	for _, c := range cases {
		a, b := normalizeField(c[0]), normalizeField(c[1])
		if a != b {
			t.Errorf("%s 与 %s 应归一化到同名,得到 %q / %q", c[0], c[1], a, b)
		}
	}
	if normalizeField("Source_Repo_Key") != normalizeField("SourceRepoKey") {
		t.Error("下划线不应影响归一化结果")
	}
}

// mapperSrc / mapperDst 是机制测试用的最小对:故意让字段名不同构。
type mapperSrc struct {
	ID          uint
	Name        string
	InstanceURL string
	GitLFS      bool
	Count       int
	CreatedAt   time.Time
	At          *time.Time
	EventID     *uint
	Secret      string
}

type mapperDst struct {
	ID          int64
	Name        string
	InstanceURL string
	GitLfs      bool
	Count       int32
	Time        string
	At          string
	EventID     *int64
	Secret      string
}

func TestMapFieldsBasics(t *testing.T) {
	tt := time.Date(2024, 5, 17, 8, 30, 15, 0, time.UTC)
	eventID := uint(91)
	src := &mapperSrc{
		ID: 11, Name: "n", InstanceURL: "https://x", GitLFS: true, Count: 7,
		CreatedAt: tt, At: &tt, EventID: &eventID, Secret: "s3cret",
	}
	spec := fieldSpec{
		layout:  layoutDateTime,
		aliases: map[string]string{"time": "createdat"},
		skip:    map[string]bool{"secret": true},
	}
	dst := &mapperDst{}
	mapFields(dst, src, spec)

	if dst.ID != 11 {
		t.Errorf("ID: got %d want 11", dst.ID)
	}
	if dst.Name != "n" {
		t.Errorf("Name: got %q want n", dst.Name)
	}
	if dst.InstanceURL != "https://x" {
		t.Errorf("InstanceURL: got %q", dst.InstanceURL)
	}
	if !dst.GitLfs {
		t.Error("GitLfs 应由 core 的 GitLFS 搬过来")
	}
	if dst.Count != 7 {
		t.Errorf("Count: got %d want 7", dst.Count)
	}
	if dst.Time != "2024-05-17 08:30:15" {
		t.Errorf("Time(别名 CreatedAt): got %q", dst.Time)
	}
	if dst.At != "2024-05-17 08:30:15" {
		t.Errorf("At: got %q", dst.At)
	}
	if dst.EventID == nil || *dst.EventID != 91 {
		t.Errorf("EventID: got %v want 91", dst.EventID)
	}
	if dst.Secret != "" {
		t.Errorf("Secret 命中 skip,不该被搬: got %q", dst.Secret)
	}
}

func TestMapFieldsNilPointerStaysZero(t *testing.T) {
	src := &mapperSrc{ID: 1}
	dst := &mapperDst{}
	mapFields(dst, src, fieldSpec{
		layout:  layoutDateTime,
		aliases: map[string]string{"time": "createdat"},
	})

	if dst.At != "" {
		t.Errorf("nil *time.Time 应保持空串: got %q", dst.At)
	}
	if dst.EventID != nil {
		t.Errorf("nil *uint 应保持 nil: got %v", dst.EventID)
	}
	// 零值 time.Time 会被格式化成零时间串(与 DTO 里 created_at 的历史行为一致)。
	if dst.Time != "0001-01-01 00:00:00" {
		t.Errorf("零值 time.Time 应格式化成零时间: got %q", dst.Time)
	}
}

func TestMapFieldsNumericClamp(t *testing.T) {
	// int → int32 钳到 [MinInt32, MaxInt32](与 SafeIntToInt32 同口径)。
	src := &mapperSrc{Count: 1 << 40}
	dst := &mapperDst{}
	mapFields(dst, src, fieldSpec{layout: layoutDateTime})
	if dst.Count != 1<<31-1 {
		t.Errorf("Count 应钳到 MaxInt32, got %d", dst.Count)
	}

	src = &mapperSrc{Count: -(1 << 40)}
	dst = &mapperDst{}
	mapFields(dst, src, fieldSpec{layout: layoutDateTime})
	if dst.Count != -(1 << 31) {
		t.Errorf("Count 应钳到 MinInt32, got %d", dst.Count)
	}
}

func TestMapFieldsSlice(t *testing.T) {
	type stepSrc struct {
		ID   uint
		Name string
	}
	type stepDst struct {
		ID   int64
		Name string
	}
	type srcS struct{ Steps []stepSrc }
	type dstS struct{ Steps []*stepDst }

	// 空切片 → DTO 保持 nil(JSON 仍是 null)。
	d := &dstS{}
	mapFields(d, &srcS{}, fieldSpec{})
	if d.Steps != nil {
		t.Errorf("空切片应保持 nil, got %v", d.Steps)
	}

	// 非空切片 → 逐元素搬。
	d = &dstS{}
	mapFields(d, &srcS{Steps: []stepSrc{{ID: 3, Name: "fetch"}}}, fieldSpec{})
	if len(d.Steps) != 1 || d.Steps[0].ID != 3 || d.Steps[0].Name != "fetch" {
		t.Errorf("切片元素未正确搬运: %+v", d.Steps)
	}
}

// TestMapFieldsPicksUpNewFieldsAutomatically 是"core 加字段只改一处"的证据:
//
// DTO 里新增的字段不需要在任何地方登记,只要 core 侧有同构字段就会自动被搬。
func TestMapFieldsPicksUpNewFieldsAutomatically(t *testing.T) {
	type src struct {
		Key           string
		ExcludeRefPat string
	}
	type dst struct {
		Key           string
		ExcludeRefPat string
	}
	d := &dst{}
	mapFields(d, &src{Key: "k", ExcludeRefPat: "refs/pull/*"}, fieldSpec{})
	if d.ExcludeRefPat != "refs/pull/*" {
		t.Errorf("新字段应自动搬运, got %q", d.ExcludeRefPat)
	}
}

// ===== 覆盖度:每个 DTO 字段都要有交代 =====

// TestFieldSpecsCoverEveryDTOField 是本次重构的安全网。
//
// 用"每个 core 字段都填上非零值"的样本跑一遍搬运,然后检查:
//   - DTO 里仍然是零值的字段,必须在契约里显式声明为 skip 或 derived;
//   - 声明了 skip 的字段,绝不能被搬过去(secret 泄漏防线)。
//
// 于是:thrift DTO 新增字段却忘了在 core 侧对应 → 这条测试立刻红。
// core 侧新增字段 → 搬运自动生效,这条测试保持绿。
func TestFieldSpecsCoverEveryDTOField(t *testing.T) {
	cases := []struct {
		name string
		src  any
		dst  any
		spec fieldSpec
	}{
		{"SyncTaskInfo", &model.SyncTask{}, &taskmodel.SyncTaskInfo{}, syncTaskInfoSpec},
		{"SyncRunInfo", &model.SyncRun{}, &taskmodel.SyncRunInfo{}, syncRunInfoSpec},
		{"SyncRunInfoLight", &model.SyncRun{}, &taskmodel.SyncRunInfo{}, syncRunInfoLightSpec},
		{"SyncRunStepInfo", &model.SyncRunStep{}, &taskmodel.SyncRunStepInfo{}, syncRunStepInfoSpec},
		{"PlatformInfo", &model.Platform{}, &platformmodel.PlatformInfo{}, platformInfoSpec},
		{"RepoInfo", &model.Repo{}, &repomodel.RepoInfo{}, repoInfoSpec},
		{"WebhookRuleInfo", &model.WebhookRule{}, &webhookmodel.WebhookRuleInfo{}, webhookRuleInfoSpec},
		{"WebhookEventInfo", &model.WebhookEvent{}, &webhookmodel.WebhookEventInfo{}, webhookEventInfoSpec},
		{"OperationLogInfo", &model.OperationLog{}, &olmodel.OperationLogInfo{}, operationLogInfoSpec},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fillStruct(reflect.ValueOf(c.src).Elem())
			mapFields(c.dst, c.src, c.spec)

			dv := reflect.ValueOf(c.dst).Elem()
			dt := dv.Type()
			for i := 0; i < dt.NumField(); i++ {
				f := dt.Field(i)
				if f.PkgPath != "" {
					continue
				}
				name := normalizeField(f.Name)
				if isZeroField(dv.Field(i)) {
					if !c.spec.skip[name] && !c.spec.derived[name] {
						t.Errorf("DTO 字段 %s 既没搬过去,也没在契约里声明 skip/derived", f.Name)
					}
					continue
				}
				if c.spec.skip[name] {
					t.Errorf("DTO 字段 %s 声明了 skip,却仍被搬运(secret 泄漏风险)", f.Name)
				}
			}
		})
	}
}

// fillStruct 给结构体每个可填字段塞一个特征非零值,让"没搬过去的字段"显形。
func fillStruct(v reflect.Value) {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" {
			continue
		}
		fillValue(v.Field(i), f.Name)
	}
}

func fillValue(v reflect.Value, name string) {
	switch v.Kind() {
	case reflect.String:
		v.SetString("x-" + name)
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(42)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(42)
	case reflect.Float32, reflect.Float64:
		v.SetFloat(1.5)
	case reflect.Struct:
		if v.Type() == reflect.TypeOf(time.Time{}) {
			v.Set(reflect.ValueOf(goldenTime()))
		}
		// 其它结构体(如 gorm.DeletedAt)保持零值:不是 DTO 字段的来源。
	case reflect.Pointer:
		et := v.Type().Elem()
		if et == reflect.TypeOf(time.Time{}) {
			tm := goldenTime()
			v.Set(reflect.ValueOf(&tm))
			return
		}
		if et.Kind() == reflect.Struct {
			return // 结构体指针保持 nil
		}
		pv := reflect.New(et)
		fillValue(pv.Elem(), name)
		v.Set(pv)
	case reflect.Slice:
		elem := reflect.New(v.Type().Elem()).Elem()
		if v.Type().Elem().Kind() == reflect.Struct {
			fillStruct(elem)
		} else {
			fillValue(elem, name)
		}
		out := reflect.MakeSlice(v.Type(), 1, 1)
		out.Index(0).Set(elem)
		v.Set(out)
	case reflect.Array:
		for i := 0; i < v.Len(); i++ {
			fillValue(v.Index(i), name)
		}
	}
}

// isZeroField 判断 DTO 字段是否仍为零值。
func isZeroField(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.String:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Pointer, reflect.Interface:
		return v.IsNil()
	case reflect.Slice:
		return v.Len() == 0
	case reflect.Struct:
		return v.IsZero()
	}
	return false
}
