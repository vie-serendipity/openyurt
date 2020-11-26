package app

import (
	restclient "k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	appsclientset "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/clientset/versioned"
)

type ControllerAppsClientBuilder interface {
	Config(name string) (*restclient.Config, error)
	ConfigOrDie(name string) *restclient.Config
	Client(name string) (appsclientset.Interface, error)
	ClientOrDie(name string) appsclientset.Interface
}

// SimpleControllerAppsClientBuilder returns a fixed client with different user agents
type SimpleControllerAppsClientBuilder struct {
	// ClientConfig is a skeleton config to clone and use as the basis for each controller client
	ClientConfig *restclient.Config
}

func (b SimpleControllerAppsClientBuilder) Config(name string) (*restclient.Config, error) {
	clientConfig := *b.ClientConfig
	return restclient.AddUserAgent(&clientConfig, name), nil
}

func (b SimpleControllerAppsClientBuilder) ConfigOrDie(name string) *restclient.Config {
	clientConfig, err := b.Config(name)
	if err != nil {
		klog.Fatal(err)
	}
	return clientConfig
}

func (b SimpleControllerAppsClientBuilder) Client(name string) (appsclientset.Interface, error) {
	clientConfig, err := b.Config(name)
	if err != nil {
		return nil, err
	}
	return appsclientset.NewForConfig(clientConfig)
}

func (b SimpleControllerAppsClientBuilder) ClientOrDie(name string) appsclientset.Interface {
	client, err := b.Client(name)
	if err != nil {
		klog.Fatal(err)
	}
	return client
}
