package pico

import (
	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/channels"
	"github.com/sipeed/picoclaw/pkg/config"
)

func init() {
	channels.RegisterFactory(
		config.ChannelPico,
		func(channelName, channelType string, cfg *config.Config, b *bus.MessageBus) (channels.Channel, error) {
			bc := cfg.Channels[channelName]
			decoded, err := bc.GetDecoded()
			if err != nil {
				return nil, err
			}
			c, ok := decoded.(*config.PicoSettings)
			if !ok {
				return nil, channels.ErrSendFailed
			}
			ch, err := NewPicoChannel(bc, c, b, cfg.Agents.Defaults.GetMaxMediaSize())
			if err != nil {
				return nil, err
			}
			if channelName != config.ChannelPico {
				ch.SetName(channelName)
			}
			// Inbound file attachments are stored in the agent workspace so
			// read_file (restricted to the workspace by default) can read them.
			ch.SetWorkspaceDir(cfg.Agents.Defaults.Workspace)
			return ch, nil
		},
	)
	channels.RegisterFactory(
		config.ChannelPicoClient,
		func(channelName, channelType string, cfg *config.Config, b *bus.MessageBus) (channels.Channel, error) {
			bc := cfg.Channels[channelName]
			decoded, err := bc.GetDecoded()
			if err != nil {
				return nil, err
			}
			c, ok := decoded.(*config.PicoClientSettings)
			if !ok {
				return nil, channels.ErrSendFailed
			}
			ch, err := NewPicoClientChannel(bc, c, b, cfg.Agents.Defaults.GetMaxMediaSize())
			if err != nil {
				return nil, err
			}
			if channelName != config.ChannelPicoClient {
				ch.SetName(channelName)
			}
			return ch, nil
		},
	)
}
