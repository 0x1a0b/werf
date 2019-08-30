package config

type ImageFromDockerfile struct {
	Name       string
	Dockerfile string
	Context    string
	Target     string
	Args       map[string]interface{}
	AddHost    []string

	raw *rawImageFromDockerfile
}

func (c *ImageFromDockerfile) GetName() string {
	return c.Name
}
