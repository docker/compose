// Copyright 2016 Apcera Inc. All rights reserved.

package server

import (
	"fmt"
	"io/ioutil"
	"reflect"
	"strings"
	"time"

	"github.com/nats-io/gnatsd/conf"
	"github.com/nats-io/nats-streaming-server/stores"
)

// ProcessConfigFile parses the configuration file `configFile` and updates
// the given Streaming options `opts`.
func ProcessConfigFile(configFile string, opts *Options) error {
	data, err := ioutil.ReadFile(configFile)
	if err != nil {
		return err
	}
	m, err := conf.Parse(string(data))
	if err != nil {
		return err
	}
	for k, v := range m {
		name := strings.ToLower(k)
		switch name {
		case "id", "cid", "cluster_id":
			if err := checkType(k, reflect.String, v); err != nil {
				return err
			}
			opts.ID = v.(string)
		case "discover_prefix":
			if err := checkType(k, reflect.String, v); err != nil {
				return err
			}
			opts.DiscoverPrefix = v.(string)
		case "st", "store_type", "store", "storetype":
			if err := checkType(k, reflect.String, v); err != nil {
				return err
			}
			switch strings.ToUpper(v.(string)) {
			case stores.TypeFile:
				opts.StoreType = stores.TypeFile
			case stores.TypeMemory:
				opts.StoreType = stores.TypeMemory
			default:
				return fmt.Errorf("Unknown store type: %v", v.(string))
			}
		case "dir", "datastore":
			if err := checkType(k, reflect.String, v); err != nil {
				return err
			}
			opts.FilestoreDir = v.(string)
		case "sd", "stan_debug":
			if err := checkType(k, reflect.Bool, v); err != nil {
				return err
			}
			opts.Debug = v.(bool)
		case "sv", "stan_trace":
			if err := checkType(k, reflect.Bool, v); err != nil {
				return err
			}
			opts.Trace = v.(bool)
		case "ns", "nats_server", "nats_server_url":
			if err := checkType(k, reflect.String, v); err != nil {
				return err
			}
			opts.NATSServerURL = v.(string)
		case "secure":
			if err := checkType(k, reflect.Bool, v); err != nil {
				return err
			}
			opts.Secure = v.(bool)
		case "tls":
			if err := parseTLS(v, opts); err != nil {
				return err
			}
		case "limits", "store_limits", "storelimits":
			if err := parseStoreLimits(v, opts); err != nil {
				return err
			}
		case "file", "file_options":
			if err := parseFileOptions(v, opts); err != nil {
				return err
			}
		}
	}
	return nil
}

// checkType returns a formatted error if `v` is not of the expected kind.
func checkType(name string, kind reflect.Kind, v interface{}) error {
	actualKind := reflect.TypeOf(v).Kind()
	if actualKind != kind {
		return fmt.Errorf("Parameter %q value is expected to be %v, got %v",
			name, kind.String(), actualKind.String())
	}
	return nil
}

// parseTLS updates `opts` with TLS config
func parseTLS(itf interface{}, opts *Options) error {
	m, ok := itf.(map[string]interface{})
	if !ok {
		return fmt.Errorf("Expected TLS to be a map/struct, got %v", itf)
	}
	for k, v := range m {
		name := strings.ToLower(k)
		switch name {
		case "client_cert":
			if err := checkType(k, reflect.String, v); err != nil {
				return err
			}
			opts.ClientCert = v.(string)
		case "client_key":
			if err := checkType(k, reflect.String, v); err != nil {
				return err
			}
			opts.ClientKey = v.(string)
		case "client_ca", "client_cacert":
			if err := checkType(k, reflect.String, v); err != nil {
				return err
			}
			opts.ClientCA = v.(string)
		}
	}
	return nil
}

// parseStoreLimits updates `opts` with store limits
func parseStoreLimits(itf interface{}, opts *Options) error {
	m, ok := itf.(map[string]interface{})
	if !ok {
		return fmt.Errorf("Expected store limits to be a map/struct, got %v", itf)
	}
	for k, v := range m {
		name := strings.ToLower(k)
		switch name {
		case "mc", "max_channels", "maxchannels":
			if err := checkType(k, reflect.Int64, v); err != nil {
				return err
			}
			opts.MaxChannels = int(v.(int64))
		case "channels", "channels_limits", "channelslimits", "per_channel", "per_channel_limits":
			if err := parsePerChannelLimits(v, opts); err != nil {
				return err
			}
		default:
			// Check for the global limits (MaxMsgs, MaxBytes, etc..)
			if err := parseChannelLimits(&opts.ChannelLimits, k, name, v); err != nil {
				return err
			}
		}
	}
	return nil
}

// parseChannelLimits updates `cl` with channel limits.
func parseChannelLimits(cl *stores.ChannelLimits, k, name string, v interface{}) error {
	switch name {
	case "msu", "max_subs", "max_subscriptions", "maxsubscriptions":
		if err := checkType(k, reflect.Int64, v); err != nil {
			return err
		}
		cl.MaxSubscriptions = int(v.(int64))
	case "mm", "max_msgs", "maxmsgs", "max_count", "maxcount":
		if err := checkType(k, reflect.Int64, v); err != nil {
			return err
		}
		cl.MaxMsgs = int(v.(int64))
	case "mb", "max_bytes", "maxbytes":
		if err := checkType(k, reflect.Int64, v); err != nil {
			return err
		}
		cl.MaxBytes = v.(int64)
	case "ma", "max_age", "maxage":
		if err := checkType(k, reflect.String, v); err != nil {
			return err
		}
		dur, err := time.ParseDuration(v.(string))
		if err != nil {
			return err
		}
		cl.MaxAge = dur
	}
	return nil
}

// parsePerChannelLimits updates `opts` with per channel limits.
func parsePerChannelLimits(itf interface{}, opts *Options) error {
	m, ok := itf.(map[string]interface{})
	if !ok {
		return fmt.Errorf("Expected per channel limits to be a map/struct, got %v", itf)
	}
	for channelName, limits := range m {
		limitsMap, ok := limits.(map[string]interface{})
		if !ok {
			return fmt.Errorf("Expected channel limits to be a map/struct, got %v", limits)
		}
		cl := &stores.ChannelLimits{}
		for k, v := range limitsMap {
			name := strings.ToLower(k)
			if err := parseChannelLimits(cl, k, name, v); err != nil {
				return err
			}
		}
		sl := &opts.StoreLimits
		sl.AddPerChannel(channelName, cl)
	}
	return nil
}

func parseFileOptions(itf interface{}, opts *Options) error {
	m, ok := itf.(map[string]interface{})
	if !ok {
		return fmt.Errorf("Expected file options to be a map/struct, got %v", itf)
	}
	for k, v := range m {
		name := strings.ToLower(k)
		switch name {
		case "compact", "compact_enabled":
			if err := checkType(k, reflect.Bool, v); err != nil {
				return err
			}
			opts.FileStoreOpts.CompactEnabled = v.(bool)
		case "compact_frag", "compact_fragmentation":
			if err := checkType(k, reflect.Int64, v); err != nil {
				return err
			}
			opts.FileStoreOpts.CompactFragmentation = int(v.(int64))
		case "compact_interval":
			if err := checkType(k, reflect.Int64, v); err != nil {
				return err
			}
			opts.FileStoreOpts.CompactInterval = int(v.(int64))
		case "compact_min_size":
			if err := checkType(k, reflect.Int64, v); err != nil {
				return err
			}
			opts.FileStoreOpts.CompactMinFileSize = v.(int64)
		case "buffer_size":
			if err := checkType(k, reflect.Int64, v); err != nil {
				return err
			}
			opts.FileStoreOpts.BufferSize = int(v.(int64))
		case "crc", "do_crc":
			if err := checkType(k, reflect.Bool, v); err != nil {
				return err
			}
			opts.FileStoreOpts.DoCRC = v.(bool)
		case "crc_poly":
			if err := checkType(k, reflect.Int64, v); err != nil {
				return err
			}
			opts.FileStoreOpts.CRCPolynomial = v.(int64)
		case "sync", "do_sync", "sync_on_flush":
			if err := checkType(k, reflect.Bool, v); err != nil {
				return err
			}
			opts.FileStoreOpts.DoSync = v.(bool)
		case "slice_max_msgs", "slice_max_count", "slice_msgs", "slice_count":
			if err := checkType(k, reflect.Int64, v); err != nil {
				return err
			}
			opts.FileStoreOpts.SliceMaxMsgs = int(v.(int64))
		case "slice_max_bytes", "slice_max_size", "slice_bytes", "slice_size":
			if err := checkType(k, reflect.Int64, v); err != nil {
				return err
			}
			opts.FileStoreOpts.SliceMaxBytes = v.(int64)
		case "slice_max_age", "slice_age", "slice_max_time", "slice_time_limit":
			if err := checkType(k, reflect.String, v); err != nil {
				return err
			}
			dur, err := time.ParseDuration(v.(string))
			if err != nil {
				return err
			}
			opts.FileStoreOpts.SliceMaxAge = dur
		case "slice_archive_script", "slice_archive", "slice_script":
			if err := checkType(k, reflect.String, v); err != nil {
				return err
			}
			opts.FileStoreOpts.SliceArchiveScript = v.(string)
		}
	}
	return nil
}
