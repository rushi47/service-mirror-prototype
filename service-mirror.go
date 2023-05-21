package main

import (
	"context"
	"flag"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	client "k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

// Make sure all global services lies in only one namespace.
const GLOBAL_SVC_NAMESPACE = "default"

type Context struct {
	ctx    context.Context
	log    logrus.Logger
	client kubernetes.Clientset
}

// Global Watch it can be of type either Service or EndpointSlice
type GlobalWatcher struct {
	ctx      Context
	Filter   labels.Selector
	informer cache.SharedInformer
	// Name space where to install service watcher,
	namespace string
}

type GlobalServiceMirrorInformers struct {
	//Service handle rinformer
	svcInformer cache.SharedInformer
	//Endpoint handler informer
	epInformer cache.SharedInformer
}

func main() {
	// Set up logger .
	log := &logrus.Logger{
		Out:   os.Stderr,
		Level: logrus.DebugLevel,
		Formatter: &logrus.TextFormatter{
			TimestampFormat: "2006-01-02 15:04:05",
			ForceColors:     true,
			DisableColors:   false,
			FullTimestamp:   true,
		},
	}
	//Shows line number: Too long
	// log.SetReportCaller(true)

	log.Info("Starting Global Mirror")

	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	}

	//Specify the NameSpace for install controller & global svc.
	globalSvcNs := flag.String("globalsvc-ns", GLOBAL_SVC_NAMESPACE, "(optional) Namespace to install service mirror controller and global mirror services.")

	flag.Parse()

	// use the current context in kubeconfig
	config, err := client.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		log.Panicf("Probably running Inside Cluster :%v", err.Error())
	}

	// creates the clientset
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Panicf("Issue in building client from config : %v", err.Error())
	}

	//Create context
	ctx := &Context{
		ctx:    context.TODO(),
		log:    *log,
		client: *client,
	}

	//Make sure it runs in loop.
	stopCh := make(chan struct{})
	defer close(stopCh)

	//Watcher for Target Services
	svcWatcher := NewServiceWatcher(*ctx, globalSvcNs)

	//Watcher for EndpointSlices of target Services
	epsWatcher := NewEndpointSlicesWatcher(*ctx, globalSvcNs)

	//Build global informer
	globaMirrorInformer := GlobalServiceMirrorInformers{
		svcInformer: svcWatcher.informer,
		epInformer:  epsWatcher.informer,
	}

	//Spint up Informer to run in thread
	go globaMirrorInformer.svcInformer.Run(stopCh)

	//Spin up endpoint informer in different thread
	go globaMirrorInformer.epInformer.Run(stopCh)

	// Wait until the informer is synced
	if !cache.WaitForCacheSync(stopCh, globaMirrorInformer.svcInformer.HasSynced, globaMirrorInformer.epInformer.HasSynced) {
		log.Panicln("Failed to sync informer cache")
	}

	// Run the program indefinitely
	<-stopCh
}
