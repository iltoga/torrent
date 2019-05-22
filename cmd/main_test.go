package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/iltoga/torrent"
	"github.com/iltoga/torrent/bencode"
	"github.com/iltoga/torrent/metainfo"
	"github.com/iltoga/torrent/tracker"
	"github.com/davecgh/go-spew/spew"
	"github.com/dustin/go-humanize"
)

var (
	builtinAnnounceList = [][]string{
		{"udp://127.0.0.1:6969/announce"},
		// {"udp://zephir.monocul.us:6969/announce"},
		// {"udp://carapax.net:6969/announce"},
		// {"udp://tracker.supertracker.net:1337/announce"},
		// {"udp://tracker.coppersurfer.tk:6969/announce"},
		// {"udp://thetracker.org:80/announce"},
	}
)

type createTorrentParams struct {
	AnnounceList   []string
	SourceFilePath string
	OutFilePath    string
}

type trackerAnnounceParams struct {
	Port     uint16
	Torrents []string
}

var baseDir = "/home/demo/tmp"
var seederPort = 50017
var leecherPort = 40017
var testTorrentName = "aa.jpg"

var args = createTorrentParams{
	SourceFilePath: baseDir + "/server/" + testTorrentName,
	OutFilePath:    baseDir + "/server/aa.torrent",
}

func Test_create_torrent_file(t *testing.T) {
	var magnet string
	var mi *metainfo.MetaInfo
	t.Run("test create .torrent from source file", func(t *testing.T) {
		clientConfig := torrent.NewDefaultClientConfig()
		clientConfig.Seed = true
		clientConfig.ListenPort = seederPort
		cl, _ := torrent.NewClient(clientConfig)

		magnet, mi, _ = makeMagnet(cl, baseDir+"/server", testTorrentName)
		outFile, err := os.Create(args.OutFilePath)
		if err != nil {
			log.Fatalf("can't create outfile: %s\n", err)
		}
		// outFile.Chmod(0644)
		defer outFile.Close()
		err = mi.Write(outFile)
		if err != nil {
			log.Fatal(err)
		}
	})

	t.Run("test tracker announce", func(t *testing.T) {
		args1 := trackerAnnounceParams{
			Port:     uint16(seederPort),
			Torrents: []string{magnet},
		}
		trackerAnnounce(args1)
	})

}

func Test_confluence(t *testing.T) {
	t.Run("test confluence post torrent metainfo", func(t *testing.T) {
		clientConfig := torrent.NewDefaultClientConfig()
		clientConfig.Seed = true
		clientConfig.ListenPort = seederPort
		cl, _ := torrent.NewClient(clientConfig)
		_, mi, _ := makeMagnet(cl, baseDir+"/server", testTorrentName)
		infoHash := mi.HashInfoBytes().HexString()

		// buck bunny
		// resp1, err := http.Get("http://127.0.0.1:8080/info?ih=dd8255ecdc7ca55fb0bbf81323d87062db1f6d1c")
		// if err != nil {
		// 	log.Fatal(err)
		// }
		// defer resp1.Body.Close()

		bodyReq, err := bencode.Marshal(mi)
		if err != nil {
			log.Fatal(err)
		}
		resp, err := http.Post("http://127.0.0.1:8080/metainfo?ih="+infoHash, "application/octet-stream", bytes.NewBuffer(bodyReq))
		if err != nil {
			log.Fatal(err)
		}
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("%v", body)

		resp2, err := http.Get("http://127.0.0.1:8080/data?ih=" + infoHash + "&path=aa.jpg")
		if err != nil {
			log.Fatal(err)
		}
		defer resp2.Body.Close()
		body, _ = ioutil.ReadAll(resp2.Body)
		log.Printf("%v", body)
	})
}

func Test_download(t *testing.T) {
	t.Run("test download external torrent", func(t *testing.T) {
		seeder, seederTorrent := createSeeder()
		defer seeder.Close()
		leecher, leecherTorrent := createLeecher()
		defer leecher.Close()
		go func() {
			for {
				st := seederTorrent.Stats()
				log.Printf("Seeder: r %v, w %v, seeders %v ", st.ConnStats.BytesRead, st.BytesWritten, st.ConnectedSeeders)
				lc := leecherTorrent.Stats()
				log.Printf("Leecher: r %v, w %v, seeders %v\n", lc.ConnStats.BytesRead, lc.BytesWritten, lc.ConnectedSeeders)
				time.Sleep(1000 * time.Millisecond)
			}
		}()

		// 	f, err := os.Create(baseDir + "/download/aa_generated.jpg")
		// 	if err != nil {
		// 		log.Fatal(err)
		// 	}
		// 	dstWriter := bufio.NewWriter(f)
		// 	done := make(chan struct{})
		// 	go func() {
		// 		defer close(done)
		// 		<-leecherTorrent.GotInfo()
		// 		for _, file := range leecherTorrent.Files() {
		// 			file.Download()
		// 			srcReader := file.NewReader()
		// 			defer srcReader.Close()
		// 			io.Copy(dstWriter, srcReader)
		// 			return
		// 		}
		// 		log.Print("file not found")
		// 	}()
		// waitDone:
		// 	for {
		// 		select {
		// 		case <-done:
		// 			break waitDone
		// 		}
		// 	}
		<-leecherTorrent.GotInfo()
		leecherTorrent.DownloadAll()
		leecher.WaitAll()
	})
}


func Test_main1(t *testing.T) {
	t.Run("test serve and download", func(t *testing.T) {
		done := make(chan struct{})
		defer close(done)
		seeder, seederTorrent := createSeeder()
		go exitSignalHandlers(seeder)

		// Write status on the root path on the default HTTP muxer. This will be
		// bound to localhost somewhere if GOPPROF is set, thanks to the envpprof
		// import.
		http.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
			seeder.WriteStatus(w)
		})

		// baseDir := "/home/demo/tmp"
		// err = seeder.StartTorrent(seederTorrent)
		// require.NoError(t, err)
		// if err != nil {
		// 	log.Fatalf("error starting seeder torrent: %s", err)
		// }

		// cfg := torrent.NewDefaultClientConfig()
		// cfg.SetListenAddr("127.0.0.1:50008")
		// cfg.DisableIPv6 = true
		// cfg.DataDir = filepath.Join(baseDir, "download")
		// cfg.DefaultStorage = storage.NewMMap(filepath.Join(baseDir, "download"))
		// defer clientConfig.DefaultStorage.Close()
		// cfg.ListenHost = torrent.LoopbackListenHost
		// cfg.NoUpload = true
		// cfg.NoDHT = false
		// clientConfig.Seed = false
		// cfg.NoDefaultPortForwarding = true
		// cfg.DisableAcceptRateLimiting = true
		// // cfg.DisableTrackers = false
		// leecher, err := torrent.NewClient(cfg)
		// require.NoError(t, err)
		// testutil.ExportStatusWriter(leecher, "l")()
		// defer leecher.Close()

		// // leecherTorrent, err := leecher.AddTorrentFromFile(args.OutFilePath)
		// metaInfo, err := metainfo.LoadFromFile(args.OutFilePath)
		// require.NoError(t, err)
		// if err != nil {
		// 	log.Fatal(err)
		// }
		// leecherTorrent, err := leecher.AddTorrent(metaInfo)
		// require.NoError(t, err)
		// if err != nil {
		// 	log.Fatal(err)
		// }
		// err = leecher.StartTorrent(leecherTorrent)
		// require.NoError(t, err)
		// if err != nil {
		// 	log.Fatalf("error starting leecher torrent: %s", err)
		// }

		// err = leecher.Start()
		// require.NoError(t, err)

		// <-leecherTorrent.GotInfo()
		// leecherTorrent.DownloadAll()
		// leecher.WaitAll()
		// log.Println("finished downloading torrent")
		// seeder.WaitAll()
		// log.Println("finished seeding torrent")

		// leecherTorrent.AddClientPeer(seeder)

		// f, err := os.Create(filepath.Join(baseDir, "download") + "recreated.jpg")
		// if err != nil {
		// 	log.Fatal(err)
		// }
		// dstWriter := bufio.NewWriter(f)
		// done := make(chan struct{})
		// 	go func() {
		// 		defer close(done)
		// 		<-leecherTorrent.GotInfo()
		// 		for _, file := range leecherTorrent.Files() {
		// 			file.Download()
		// 			srcReader := file.NewReader()
		// 			defer srcReader.Close()
		// 			io.Copy(dstWriter, srcReader)
		// 			return
		// 		}
		// 		log.Print("file not found")
		// 	}()
		ticker := time.NewTicker(time.Second)
	waitDone:
		for {
			select {
			case <-done:
				break waitDone
			case <-ticker.C:
				// os.Stdout.WriteString(progressLine(seeder))
			}
		}
		if seederTorrent.Seeding() {
			select {}
		}
	})

}

func argSpec(arg string) (ts *torrent.TorrentSpec, err error) {
	if strings.HasPrefix(arg, "magnet:") {
		return torrent.TorrentSpecFromMagnetURI(arg)
	}
	mi, err := metainfo.LoadFromFile(arg)
	if err != nil {
		return
	}
	ts = torrent.TorrentSpecFromMetaInfo(mi)
	return
}

func doTracker(tURI string, done func(), ar tracker.AnnounceRequest) {
	defer done()
	for _, res := range announces(tURI, ar) {
		err := res.error
		resp := res.AnnounceResponse
		if err != nil {
			log.Printf("error announcing to %q: %s", tURI, err)
			continue
		}
		log.Printf("tracker response from %q: %s", tURI, spew.Sdump(resp))
	}
}

type announceResult struct {
	tracker.AnnounceResponse
	error
}

func announces(uri string, ar tracker.AnnounceRequest) (ret []announceResult) {
	u, err := url.Parse(uri)
	if err != nil {
		return []announceResult{{error: err}}
	}
	a := tracker.Announce{
		Request:    ar,
		TrackerUrl: uri,
	}
	if u.Scheme == "udp" {
		a.UdpNetwork = "udp4"
		ret = append(ret, announce(a))
		a.UdpNetwork = "udp6"
		ret = append(ret, announce(a))
		return
	}
	return []announceResult{announce(a)}
}

func announce(a tracker.Announce) announceResult {
	resp, err := a.Do()
	return announceResult{resp, err}
}

func exitSignalHandlers(client *torrent.Client) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	for {
		log.Printf("close signal received: %+v", <-c)
		client.Close()
	}
}

func bytesCompleted(tc *torrent.Client) (ret int64) {
	for _, t := range tc.Torrents() {
		if t.Info() != nil {
			ret += t.BytesCompleted()
		}
	}
	return
}

// Returns an estimate of the total bytes for all torrents.
func totalBytesEstimate(tc *torrent.Client) (ret int64) {
	var noInfo, hadInfo int64
	for _, t := range tc.Torrents() {
		info := t.Info()
		if info == nil {
			noInfo++
			continue
		}
		ret += info.TotalLength()
		hadInfo++
	}
	if hadInfo != 0 {
		// Treat each torrent without info as the average of those with,
		// rounded up.
		ret += (noInfo*ret + hadInfo - 1) / hadInfo
	}
	return
}

func progressLine(tc *torrent.Client) string {
	return fmt.Sprintf("\033[K%s / %s\r", humanize.Bytes(uint64(bytesCompleted(tc))), humanize.Bytes(uint64(totalBytesEstimate(tc))))
}

func makeMagnet(cl *torrent.Client, dir string, name string) (string, *metainfo.MetaInfo, *torrent.Torrent) {
	mi := metainfo.MetaInfo{
		AnnounceList: builtinAnnounceList,
		Comment:      "bcz snapshot",
		CreatedBy:    "bcz spine-chain prototype",
		CreationDate: time.Now().Unix(),
	}
	for _, a := range args.AnnounceList {
		mi.AnnounceList = append(mi.AnnounceList, []string{a})
	}
	path := filepath.Join(dir, name)
	piecesLength := calculatePieceLengthFromFile(path)
	info := metainfo.Info{PieceLength: piecesLength}
	err := info.BuildFromFilePath(path)
	if err != nil {
		log.Fatal(err)
	}
	mi.InfoBytes, err = bencode.Marshal(info)
	if err != nil {
		log.Fatal(err)
	}
	magnet := mi.Magnet(name, mi.HashInfoBytes()).String()
	tr, err := cl.AddTorrent(&mi)
	if err != nil {
		log.Fatal(err)
	}
	tr.VerifyData()
	return magnet, &mi, tr
}

func calculatePieceLengthFromFile(path string) int64 {
	f, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		log.Fatal(err)
	}
	switch {
	case fi.Size() < 1000*1024:
		// 10 pieces
		return int64(float64(fi.Size() / 10))
	case fi.Size() < 10000*1024:
		// 20 pieces
		return int64(float64(fi.Size() / 20))
	default:
		// 50 pieces
		return int64(float64(fi.Size() / 50))
	}
}

func trackerAnnounce(params trackerAnnounceParams) {
	ar := tracker.AnnounceRequest{
		NumWant: -1,
		Left:    math.MaxUint64,
		Port:    params.Port,
	}
	var wg sync.WaitGroup
	for _, arg := range params.Torrents {
		ts, err := argSpec(arg)
		if err != nil {
			log.Fatal(err)
		}
		ar.InfoHash = ts.InfoHash
		for _, tier := range ts.Trackers {
			for _, tURI := range tier {
				wg.Add(1)
				go doTracker(tURI, wg.Done, ar)
			}
		}
	}
	wg.Wait()
	log.Println("done announcing torrent to given Trackers")

}

func createSeeder() (*torrent.Client, *torrent.Torrent) {
	clientConfig := torrent.NewDefaultClientConfig()
	// we are a seeder, so provide location of full torrent file
	clientConfig.DataDir = filepath.Join(baseDir, "server")
	// clientConfig.DefaultStorage = storage.NewMMap(filepath.Join(baseDir, "seederstorage"))
	// defer clientConfig.DefaultStorage.Close()
	clientConfig.ListenPort = seederPort
	// clientConfig.PublicIp4 = net.ParseIP("36.83.156.39")
	clientConfig.DisableIPv6 = true
	clientConfig.DisableTrackers = false
	clientConfig.NoDHT = true
	clientConfig.Seed = true
	seeder, err := torrent.NewClient(clientConfig)
	if err != nil {
		log.Fatalf("error creating seeder: %s\n", err)
	}
	// defer seeder.Close()
	// defer testutil.ExportStatusWriter(seeder, "s")()

	// add torrent to seeder
	magnet, _, seederTorrent := makeMagnet(seeder, baseDir+"/server", testTorrentName)
	if !seederTorrent.Seeding() {
		log.Fatalf("seeder not seeding: %s\n", err)
	}
	// already in makeMagnet
	// seederTorrent.VerifyData()

	seederArgs := trackerAnnounceParams{
		Port:     uint16(seederPort),
		Torrents: []string{magnet},
	}
	// announce this torrent to trackers
	trackerAnnounce(seederArgs)

	return seeder, seederTorrent
}

func createLeecher() (*torrent.Client, *torrent.Torrent) {
	clientConfig := torrent.NewDefaultClientConfig()
	// we are a seeder, so provide location of full torrent file
	clientConfig.DataDir = filepath.Join(baseDir, "client")
	// clientConfig.DefaultStorage = storage.NewMMap(filepath.Join(baseDir, "leecherstorage"))
	// defer clientConfig.DefaultStorage.Close()
	clientConfig.ListenPort = leecherPort
	// clientConfig.PublicIp4 = net.ParseIP("36.83.156.39")
	clientConfig.DisableIPv6 = true
	clientConfig.DisableTrackers = false
	clientConfig.NoDHT = true
	clientConfig.Seed = false
	clientConfig.NoUpload = true

	leecher, err := torrent.NewClient(clientConfig)
	if err != nil {
		log.Fatalf("error creating leecher: %s\n", err)
	}
	// defer leecher.Close()
	// defer testutil.ExportStatusWriter(leecher, "s")()

	// add torrent to leecher
	magnet, _, leecherTorrent := makeMagnet(leecher, baseDir+"/server", testTorrentName)
	if leecherTorrent.Seeding() {
		log.Fatalf("leecher seeding: %s\n", err)
	}
	// already in makeMagnet
	// leecherTorrent.VerifyData()

	leecherArgs := trackerAnnounceParams{
		Port:     uint16(leecherPort),
		Torrents: []string{magnet},
	}
	// announce this torrent to trackers
	trackerAnnounce(leecherArgs)

	return leecher, leecherTorrent
}
