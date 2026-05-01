package memory

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/yourname/dynamic_memory_go/proto/pb"
	"gorm.io/gorm"
)

// MemoryLevel 记忆层级定义
type MemoryLevel int

const (
	L0 MemoryLevel = iota // 工作记忆：热存储（Redis）
	L1                     // 情景记忆：温存储（MySQL）
	L2                     // 语义记忆：温存储（MySQL）
	L3                     // 核心记忆：只读持久化存储
)

// Memory 记忆数据结构（通用意义存储，不存储原始冗余文本）
type Memory struct {
	ID        string      `json:"id"`         // 唯一记忆ID
	Level     MemoryLevel `json:"level"`      // 记忆层级
	Content   string      `json:"content"`    // 语义摘要（非原始输入）
	Vector    []float32   `json:"vector"`     // 语义向量（用于关联检索）
	Weight    float64     `json:"weight"`     // 重要性权重(0-1)
	Tags      []string    `json:"tags"`       // 关联标签
	CreatedAt time.Time   `json:"created_at"` // 创建时间
	UpdatedAt time.Time   `json:"updated_at"` // 最后激活时间
	IsSleep   bool        `json:"is_sleep"`   // 是否休眠（冷存储标记）
}

// CoreMemory 核心记忆结构（永不遗忘的核心配置/身份）
type CoreMemory struct {
	CoreIdentity string   `json:"core_identity"` // 核心身份/配置（只读）
}

var (
	rdb         *redis.Client
	db          *gorm.DB
	metaClient  pb.MemoryMetaServiceClient
	coreTags    []string
	levelConfig map[string]interface{}
	coldPath    string
)

// 初始化整个记忆系统，加载配置、连存储、连元数据服务
func InitMemory(configPath string) error {
	// 加载配置
	viper.SetConfigFile(configPath)
	if err := viper.ReadInConfig(); err != nil {
		return fmt.Errorf("read config failed: %w", err)
	}

	// 读配置项
	coreTags = viper.GetStringSlice("core_tags")
	levelConfig = viper.GetStringMap("levels")
	coldPath = viper.GetString("storage.cold_storage_path")

	// 连Redis
	redisAddr := viper.GetString("storage.redis.addr")
	redisPass := viper.GetString("storage.redis.password")
	redisDB := viper.GetInt("storage.redis.db")
	rdb = redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: redisPass,
		DB:       redisDB,
	})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		return fmt.Errorf("redis connect failed: %w", err)
	}

	// 连MySQL
	mysqlDSN := viper.GetString("storage.mysql.dsn")
	var err error
	db, err = gorm.Open(mysql.Open(mysqlDSN), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("mysql connect failed: %w", err)
	}
	// 自动建表
	db.AutoMigrate(&Memory{})

	// 连元数据服务
	metaAddr := viper.GetString("meta_service.addr")
	conn, err := grpc.Dial(metaAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("meta service connect failed: %w", err)
	}
	metaClient = pb.NewMemoryMetaServiceClient(conn)

	// 加载核心记忆
	loadCoreMemory()

	// 冷存储目录，需要的话打开
	// os.MkdirAll(coldPath, 0755)

	return nil
}

// 加新记忆的入口，进来先筛一遍，再分层存
func AddMemory(ctx context.Context, rawInput string) error {
	// 先调元数据服务算一下这个输入的权重、摘要这些
	metaResp, err := metaClient.CalculateMemoryMeta(ctx, &pb.MemoryMetaRequest{RawInput: rawInput})
	if err != nil {
		return err
	}

	// 太低权重的，就存个极简记录就行，别占地方
	lightThreshold := viper.GetFloat64("levels.L0.light_weight_threshold")
	if metaResp.Weight < float32(lightThreshold) {
		lightMem := &Memory{
			Level:   L0,
			Content: "日常互动记录",
			Weight:  float64(metaResp.Weight),
			Tags:    []string{"daily"},
		}
		return saveL0(lightMem)
	}

	// 构建记忆，不存原始输入，只存摘要
	mem := &Memory{
		ID:        generateMemID(),
		Level:     L0, // 新的先放工作记忆
		Content:   metaResp.SemanticSummary,
		Vector:    metaResp.Vector,
		Weight:    float64(metaResp.Weight),
		Tags:      metaResp.Tags,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		IsSleep:   false,
	}

	// 存到L0
	if err := saveL0(mem); err != nil {
		return err
	}

	// 权重够高的，直接异步升级到L1
	upgradeThreshold := viper.GetFloat64("levels.L0.upgrade_weight_threshold")
	if mem.Weight > upgradeThreshold {
		go upgradeToL1(mem.ID)
	}

	// 核心标签的，记个核心日志
	if containsAny(mem.Tags, coreTags) {
		go appendCoreLog(fmt.Sprintf("[%s] 核心互动: %s", time.Now().Format("2006-01-02 15:04"), mem.Content))
	}

	return nil
}

// 查记忆，对话前调用，把相关的老记忆找出来
func RetrieveMemory(ctx context.Context, currentInput string) ([]*Memory, error) {
	// 先拿当前输入的向量
	vectorResp, err := metaClient.ExtractVector(ctx, &pb.VectorRequest{Input: currentInput})
	if err != nil {
		return nil, err
	}
	currentVector := vectorResp.Vector

	// 从各个层级查相关的
	memories := []*Memory{}
	// 先查L0的热数据
	l0Mems, err := queryL0ByVector(currentVector)
	if err == nil {
		memories = append(memories, l0Mems...)
	}
	// 再查L1/L2的温数据
	l1l2Mems, err := queryL1L2ByVector(currentVector)
	if err == nil {
		memories = append(memories, l1l2Mems...)
	}

	// 异步看看有没有休眠的记忆能唤醒的
	go tryWakeSleepMemories(currentVector)

	// 排个序，取最相关的前10个
	return sortMemoriesByRelevance(memories, currentVector)[:10], nil
}

// 定时整理记忆，每天跑一次，把该升级、该休眠的都处理了
func SleepMemory() {
	// 处理L0的
	l0Memories := getAllL0()
	l0MaxItems := viper.GetInt("levels.L0.max_items")
	l0ExpireDays := viper.GetInt("levels.L0.expire_days")
	l0UpgradeThreshold := viper.GetFloat64("levels.L0.upgrade_weight_threshold")

	for _, mem := range l0Memories {
		if time.Since(mem.CreatedAt) > time.Duration(l0ExpireDays)*24*time.Hour || len(l0Memories) > l0MaxItems {
			if mem.Weight > l0UpgradeThreshold {
				upgradeToL1(mem.ID)
			} else {
				sleepMemory(mem.ID)
			}
		}
	}

	// 处理L1的
	l1Memories := getAllL1()
	l1ExpireDays := viper.GetInt("levels.L1.expire_days")
	l1SleepThreshold := viper.GetFloat64("levels.L1.sleep_weight_threshold")
	l1IntegrateThreshold := viper.GetFloat64("levels.L1.integrate_weight_threshold")

	for _, mem := range l1Memories {
		if mem.Weight < l1SleepThreshold || time.Since(mem.UpdatedAt) > time.Duration(l1ExpireDays)*24*time.Hour {
			sleepMemory(mem.ID)
		}
		if mem.Weight > l1IntegrateThreshold {
			go triggerIntegrateToL2(mem.ID)
		}
	}

	// 处理L2的
	l2Memories := getAllL2()
	l2ExpireDays := viper.GetInt("levels.L2.expire_days")
	l2SleepThreshold := viper.GetFloat64("levels.L2.sleep_weight_threshold")

	for _, mem := range l2Memories {
		if mem.Weight < l2SleepThreshold || time.Since(mem.UpdatedAt) > time.Duration(l2ExpireDays)*24*time.Hour {
			sleepMemory(mem.ID)
		}
	}
}

// ========== 内部的辅助函数，你可以自己补实现 ==========
func loadCoreMemory() error {
	// 加载核心记忆到内存，只读的
	return nil
}

func generateMemID() string {
	// 生成个唯一的记忆ID
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func saveL0(mem *Memory) error {
	// 写到Redis，示例你可以参考这个改：
	// key := fmt.Sprintf("l0:mem:%s", mem.ID)
	// data, _ := json.Marshal(mem)
	// return rdb.Set(ctx, key, data, 7*24*time.Hour).Err()
	return nil
}

func getAllL0() []*Memory {
	// 拿所有L0的记忆
	return nil
}

func queryL0ByVector(vector []float32) ([]*Memory, error) {
	// 按向量相似度查L0的记忆
	return nil, nil
}

func upgradeToL1(memID string) error {
	// 把记忆从Redis挪到MySQL，层级改成L1
	// 示例：
	// mem, err := getL0ByID(memID)
	// if err != nil {
	//     return err
	// }
	// mem.Level = L1
	// return db.Create(mem).Error
	return nil
}

func getAllL1() []*Memory {
	// 拿所有L1的记忆
	return nil
}

func getAllL2() []*Memory {
	// 拿所有L2的记忆
	return nil
}

func queryL1L2ByVector(vector []float32) ([]*Memory, error) {
	// 按向量相似度查L1/L2的记忆
	return nil, nil
}

func sleepMemory(memID string) error {
	// 把记忆从热/温存储挪到冷存储，标记成休眠
	return nil
}

func tryWakeSleepMemories(vector []float32) error {
	// 看看冷存储里有没有强关联的，有的话唤醒
	return nil
}

func triggerIntegrateToL2(memID string) error {
	// 调元数据服务，把相似的L1记忆整合成L2的
	return nil
}

func appendCoreLog(log string) error {
	// 记个核心日志
	return nil
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func containsAny(slice []string, items []string) bool {
	for _, s := range slice {
		for _, item := range items {
			if s == item {
				return true
			}
		}
	}
	return false
}

func sortMemoriesByRelevance(mems []*Memory, vector []float32) ([]*Memory, error) {
	// 按相似度+权重排个序
	return mems, nil
}
