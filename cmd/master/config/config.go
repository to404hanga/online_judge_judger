package config

type LRUConfig struct {
	Size int `yaml:"size"` // 缓存中可容纳的项数
}

func (LRUConfig) Key() string {
	return "lru"
}
