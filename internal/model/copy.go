package model

// Copy 返回 LocalConfig 的深拷贝
func (c *LocalConfig) Copy() *LocalConfig {
	if c == nil {
		return nil
	}
	out := &LocalConfig{
		MinecraftDir: c.MinecraftDir,
		ServerURL:    c.ServerURL,
		ServerToken:  c.ServerToken,
		Launcher:     c.Launcher,
		JavaHome:     c.JavaHome,
		Username:     c.Username,
		MirrorMode:   c.MirrorMode,
	}
	if c.MinecraftDirs != nil {
		out.MinecraftDirs = make(map[string]string, len(c.MinecraftDirs))
		for k, v := range c.MinecraftDirs {
			out.MinecraftDirs[k] = v
		}
	}
	if c.Packs != nil {
		out.Packs = make(map[string]PackState, len(c.Packs))
		for k, v := range c.Packs {
			cp := v
			if v.Channels != nil {
				cp.Channels = make(map[string]ChannelState, len(v.Channels))
				for ck, cv := range v.Channels {
					cp.Channels[ck] = cv
				}
			}
			out.Packs[k] = cp
		}
	}
	return out
}
