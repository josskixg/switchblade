package providers

// Registry holds providers in priority order. First match wins.
type Registry struct {
	providers   []Provider
	ConfigStore *ConfigStore
}

// Register appends a provider and calls Configure if the provider supports it.
func (r *Registry) Register(p Provider) {
	if r.ConfigStore != nil {
		if c, ok := p.(Configurable); ok {
			c.Configure(r.ConfigStore)
		}
	}
	r.providers = append(r.providers, p)
}

// Route returns the first provider that claims the model, or nil.
func (r *Registry) Route(model string) Provider {
	for _, p := range r.providers {
		if r.ConfigStore != nil {
			if configured, ok := r.ConfigStore.Models(p.Name()); ok {
				if !r.ConfigStore.Get(p.Name()).Enabled {
					continue
				}
				for _, candidate := range configured {
					if candidate == model {
						return p
					}
				}
				continue
			}
		}
		if p.OwnsModel(model) {
			return p
		}
	}
	return nil
}

// All returns all registered providers in priority order.
func (r *Registry) All() []Provider {
	return r.providers
}
