package discord

import (
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"
)

func TestHelpDataStructure(t *testing.T) {
	data := getHelpData()
	foundGames := false
	foundSystem := false
	walletCmds := 0

	for _, cat := range data {
		if cat.Name == "games" {
			foundGames = true
		}
		if cat.Name == "system" {
			foundSystem = true
		}
		if cat.Name == "wallet" {
			walletCmds = len(cat.Commands)
		}
	}

	if !foundGames {
		t.Error("helpData is missing 'games' category")
	}
	if !foundSystem {
		t.Error("helpData is missing 'system' category")
	}
	if walletCmds == 0 {
		t.Error("'wallet' category has no commands")
	}
}

func TestRenderCommandsHaveExpectedHelpPermissions(t *testing.T) {
	roles := make(map[string]RoleLevel)
	for _, category := range getHelpData() {
		for _, command := range category.Commands {
			roles[command.Name] = command.MinRole
		}
	}
	if roles["!render"] != RoleAdmin {
		t.Fatalf("!render should be available to Admin, got role %v", roles["!render"])
	}
	if roles["!upd"] != RoleOwner {
		t.Fatalf("!upd should be Owner-only, got role %v", roles["!upd"])
	}
}

func TestBuildHelpCategoryEmbed(t *testing.T) {
	embed := buildHelpCategoryEmbed("wallet", RoleUser, "!")
	if !strings.Contains(embed.Title, "Tài Sản") {
		t.Errorf("Expected title to contain 'Tài Sản', got %s", embed.Title)
	}
	if !strings.Contains(embed.Description, "`!pay @user <số tiền>`") {
		t.Errorf("Embed description is missing a command, got: %s", embed.Description)
	}
}

func TestBuildHelpComponents_Permissions(t *testing.T) {
	b := &Bot{}

	t.Run("User Role", func(t *testing.T) {
		components := b.buildHelpComponents(nil, RoleUser, "")
		selectMenu := components[0].(discordgo.ActionsRow).Components[0].(discordgo.SelectMenu)
		for _, opt := range selectMenu.Options {
			if opt.Value == "system" || opt.Value == "owner" {
				t.Error("User can see admin/system categories")
			}
		}
	})

	t.Run("Owner Role", func(t *testing.T) {
		components := b.buildHelpComponents(nil, RoleOwner, "")
		selectMenu := components[0].(discordgo.ActionsRow).Components[0].(discordgo.SelectMenu)
		if len(selectMenu.Options) < 5 {
			t.Errorf("Owner does not see all 5 categories, got %d", len(selectMenu.Options))
		}
		buttonRow := components[1].(discordgo.ActionsRow)
		prefixButton := buttonRow.Components[0].(discordgo.Button)
		if prefixButton.CustomID != "help:set_prefix" || prefixButton.Label != "Đổi Prefix" || prefixButton.Disabled {
			t.Errorf("Owner main menu should have an enabled change-prefix button, got %+v", prefixButton)
		}
	})

	t.Run("Category Back Button", func(t *testing.T) {
		components := b.buildHelpComponents(nil, RoleOwner, "wallet")
		buttonRow := components[1].(discordgo.ActionsRow)
		backButton := buttonRow.Components[0].(discordgo.Button)
		if backButton.CustomID != "help:back_to_main" || backButton.Label != "Quay Lại" || backButton.Disabled {
			t.Errorf("Category menu should have an enabled back button, got %+v", backButton)
		}
	})

	t.Run("Non-owner Cannot Change Prefix", func(t *testing.T) {
		components := b.buildHelpComponents(nil, RoleAdmin, "")
		buttonRow := components[1].(discordgo.ActionsRow)
		button := buttonRow.Components[0].(discordgo.Button)
		if button.CustomID != "help:set_prefix" || button.Label != "Đổi Prefix" || !button.Disabled {
			t.Errorf("Non-owner main menu should show a disabled change-prefix button, got %+v", button)
		}
	})

	t.Run("Invite Button", func(t *testing.T) {
		mockSession := &discordgo.Session{
			State: &discordgo.State{},
		}
		mockSession.State.User = &discordgo.User{ID: "mock_bot_id"}
		components := b.buildHelpComponents(mockSession, RoleUser, "")
		buttonRow := components[1].(discordgo.ActionsRow)
		found := false
		for _, comp := range buttonRow.Components {
			if btn, ok := comp.(discordgo.Button); ok && btn.Style == discordgo.LinkButton {
				found = true
				if !strings.Contains(btn.URL, "client_id=mock_bot_id") ||
					!strings.Contains(btn.URL, "scope=bot%20applications.commands") {
					t.Error("Invite button has wrong URL")
				}
			}
		}
		if !found {
			t.Error("Invite button not found")
		}
	})
}
