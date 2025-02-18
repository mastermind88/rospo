package cmd

import (
	"log"
	"os"
	"os/signal"

	"github.com/ferama/rospo/pkg/conf"
	"github.com/ferama/rospo/pkg/sshc"
	"github.com/ferama/rospo/pkg/sshd"
	"github.com/ferama/rospo/pkg/tun"
	"github.com/ferama/rospo/pkg/web"
	rootapi "github.com/ferama/rospo/pkg/web/api/root"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(runCmd)
}

var runCmd = &cobra.Command{
	Use:   "run config_file_path.yaml",
	Short: "Run rospo using a config file.",
	Long:  "Run rospo using a config file.",
	Args:  cobra.MinimumNArgs(1),
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) != 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return []string{"yaml"}, cobra.ShellCompDirectiveFilterFileExt
	},
	Run: func(cmd *cobra.Command, args []string) {
		conf, err := conf.LoadConfig(args[0])
		if err != nil {
			log.Fatalln(err)
		}
		somethingRun := false

		var sshConn *sshc.SshConnection

		if conf.SshClient != nil {
			sshConn = sshc.NewSshConnection(conf.SshClient)
			go sshConn.Start()
			somethingRun = true
		}

		failIfNoClient := func(item string) {
			if sshConn == nil {
				log.Fatalf("you need to configure sshclient section to support %s", item)
			}
		}

		if conf.SshD != nil {
			sshServer := sshd.NewSshServer(conf.SshD)
			go sshServer.Start()
			somethingRun = true
		}

		if conf.Tunnel != nil && len(conf.Tunnel) > 0 {
			for _, c := range conf.Tunnel {
				if c.SshClientConf != nil {
					conn := sshc.NewSshConnection(c.SshClientConf)
					go conn.Start()
					go tun.NewTunnel(conn, c, false).Start()
				} else {
					failIfNoClient("tunnel")
					go tun.NewTunnel(sshConn, c, false).Start()
				}
			}
		}

		if conf.Web != nil {
			failIfNoClient("web api")

			dev := false
			if Version == "development" {
				dev = true
			}
			jh := []string{}
			info := &rootapi.Info{}
			if conf.SshClient != nil {
				for _, h := range conf.SshClient.JumpHosts {
					jh = append(jh, h.URI)
				}
				info = &rootapi.Info{
					SshClientURI: conf.SshClient.ServerURI,
					JumpHosts:    jh,
				}
			}

			go web.StartServer(dev, sshConn, conf.Web, info)
		}

		if conf.SocksProxy != nil {
			var sockProxy *sshc.SocksProxy
			if conf.SocksProxy.SshClientConf == nil {
				failIfNoClient("socks proxy")
				sockProxy = sshc.NewSocksProxy(sshConn)
			} else {
				proxySshConn := sshc.NewSshConnection(conf.SocksProxy.SshClientConf)
				go proxySshConn.Start()
				sockProxy = sshc.NewSocksProxy(proxySshConn)
			}
			somethingRun = true

			go func() {
				err := sockProxy.Start(conf.SocksProxy.ListenAddress)
				if err != nil {
					log.Fatal(err)
				}
			}()
		}

		if somethingRun {
			c := make(chan os.Signal, 1)
			signal.Notify(c, os.Interrupt)
			<-c
		} else {
			log.Println("nothing to run")
		}
	},
}
