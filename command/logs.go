package command

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/spf13/pflag"

	"github.com/spf13/cobra"
)

// LogsOptions LogsOptions
type LogsOptions struct {
	// PodLogOptions
	SinceTime    string
	SinceSeconds time.Duration
	Follow       bool
	Previous     bool
	Timestamps   bool
	LimitBytes   int64
	Tail         int64
	Container    string

	// All
	Namespace string
	Debug     bool
}

// Usage Usage
const Usage string = "kubelogs [-f] [-p] (POD | TYPE/NAME) [-c CONTAINER]"
const logsExample = `kubelogs my-pod-v1
  kubelogs my-pod-v1 -c my-container
  kubelogs regex -f
  kubelogs my-pod-v1 --since 10m
  kubelogs --tail 1`

// NewLogsOptions NewLogsOptions
func NewLogsOptions() *LogsOptions {
	return &LogsOptions{
		Tail:      -1,
		Namespace: "default",
	}
}

// NewCmdLogs creates a new pod logs command
func NewCmdLogs() *cobra.Command {
	logrus.SetOutput(os.Stdout)
	logrus.SetLevel(logrus.InfoLevel)

	o := NewLogsOptions()

	cmd := &cobra.Command{
		Use:                   Usage,
		DisableFlagsInUseLine: true,
		Short:                 `Print the logs for a container in a pod`,
		Long:                  `Print the logs for a container in a pod or specified resource. If the pod has only one container, the container name is optional.`,
		Example:               logsExample,
		Run: func(cmd *cobra.Command, args []string) {
			if o.Debug {
				logrus.SetLevel(logrus.TraceLevel)
			}

			fg := ""
			cmd.Flags().VisitAll(func(flag *pflag.Flag) {
				if flag.Name == "help" || flag.Name == "debug" || !flag.Changed {
					return
				}
				fg += fmt.Sprintf(" --%s=%s", flag.Name, flag.Value)
			})

			pods := new(Pods)
			for _, arg := range args {
				p, err := cmdGetPods(arg, o.Container, o.Namespace)
				if err != nil {
					logrus.Fatalln(err)
				}

				pods.Items = append(pods.Items, p.Items...)
			}

			logrus.Infoln(fmt.Sprintf("kubelogs for %d pod", len(pods.Items)))

			wg := &sync.WaitGroup{}
			for _, pod := range pods.Items {
				for _, container := range pod.Containers {
					str := fmt.Sprintf("kubectl logs %s %s", pod.Name, fg)
					if o.Container == "" {
						str += fmt.Sprintf(" --container %s", container.Name)
					}
					logrus.Infoln(fmt.Sprintf("%s %s", pod.Name, container.Name))
					logrus.Debugln(str)

					prefix := fmt.Sprintf("[%s %s]", pod.Name, container.Name)

					wg.Add(1)
					go func(str, prefix string) {
						defer wg.Done()

						command := exec.Command("bash", "-c", str)
						stdout, err := command.StdoutPipe()
						if err != nil {
							logrus.Errorln(err)
							return
						}
						stderr, err := command.StderrPipe()
						if err != nil {
							logrus.Errorln(err)
							return
						}

						if err := command.Start(); err != nil {
							logrus.Errorln(err)
							return
						}

						go func() {
							reader := bufio.NewReader(stdout)

							for {
								line, err := reader.ReadString('\n')
								if err != nil || io.EOF == err {
									break
								}
								logrus.Infoln(prefix + line)
							}
						}()

						go func() {
							reader := bufio.NewReader(stderr)

							for {
								line, err := reader.ReadString('\n')
								if err != nil || io.EOF == err {
									break
								}
								logrus.Infoln(prefix + line)
							}
						}()

						if err := command.Wait(); err != nil {
							logrus.Errorln(err)
							return
						}

						logrus.Infoln(prefix + "exit")
					}(str, prefix)
				}
			}

			wg.Wait()
		},
	}

	cmd.Flags().BoolVarP(&o.Follow, "follow", "f", o.Follow, "Specify if the logs should be streamed.")
	cmd.Flags().BoolVar(&o.Timestamps, "timestamps", o.Timestamps, "Include timestamps on each line in the log output")
	cmd.Flags().Int64Var(&o.LimitBytes, "limit-bytes", o.LimitBytes, "Maximum bytes of logs to return. Defaults to no limit.")
	cmd.Flags().BoolVarP(&o.Previous, "previous", "p", o.Previous, "If true, print the logs for the previous instance of the container in a pod if it exists.")
	cmd.Flags().Int64Var(&o.Tail, "tail", o.Tail, "Lines of recent log file to display. Showing all log lines otherwise 10, if a selector is provided.")
	cmd.Flags().StringVar(&o.SinceTime, "since-time", o.SinceTime, "Only return logs after a specific date (RFC3339). Defaults to all logs. Only one of since-time / since may be used.")
	cmd.Flags().DurationVar(&o.SinceSeconds, "since", o.SinceSeconds, "Only return logs newer than a relative duration like 5s, 2m, or 3h. Defaults to all logs. Only one of since-time / since may be used.")
	cmd.Flags().StringVarP(&o.Container, "container", "c", o.Container, "Print the logs of this container")

	cmd.Flags().StringVarP(&o.Namespace, "namespace", "n", o.Namespace, `The Kubernetes namespace where the pods are located`)
	cmd.Flags().BoolVarP(&o.Debug, "debug", "v", o.Debug, `Debug tool`)

	return cmd
}

func cmdGetPods(podRegexp, cName, namespace string) (pods *Pods, err error) {
	cmd := exec.Command("bash", "-c", fmt.Sprintf(`kubectl get pod -n %s --output=jsonpath="{range .items[*]}{.metadata.name} {.spec['containers', 'initContainers'][*].name}|{end}"`, namespace))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return
	}

	pr, err := regexp.Compile(podRegexp)
	if err != nil {
		logrus.Fatalln(err)
	}

	pods = new(Pods)

	ps := strings.Split(string(out), "|")
	for _, p := range ps {
		cs := strings.Split(string(p), " ")
		if !pr.MatchString(cs[0]) {
			continue
		}

		pod := &Pod{
			Name:       cs[0],
			Containers: make([]*Container, 0, len(cs[1:])),
		}
		for _, c := range cs[1:] {
			if cName != "" && cName != c {
				continue
			}
			pod.Containers = append(pod.Containers, &Container{
				Name: c,
			})
		}

		pods.Items = append(pods.Items, pod)
	}

	return
}

// Pods Pods
type Pods struct {
	Items []*Pod
}

// Pod Pod
type Pod struct {
	Name       string
	Containers []*Container
}

// Container Container
type Container struct {
	Name string
}
