import { useState } from "react";
import { useVpn } from "./state/useVpn";
import { useAccount } from "./account/useAccount";
import { StatusBar } from "./components/StatusBar";
import { Sidebar } from "./components/Sidebar";
import { WorldMap } from "./components/WorldMap";
import { SettingsPanel } from "./components/SettingsPanel";
import { LoginScreen } from "./components/LoginScreen";
import { ProfilePanel } from "./components/ProfilePanel";
import { SubscriptionDialog } from "./components/SubscriptionDialog";

export default function App() {
  const acct = useAccount();

  if (!acct.loggedIn) {
    return <LoginScreen onLogin={acct.login} />;
  }
  return <MainApp acct={acct} />;
}

function MainApp({ acct }: { acct: ReturnType<typeof useAccount> }) {
  const vpn = useVpn();
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [profileOpen, setProfileOpen] = useState(false);
  const [focusRedeem, setFocusRedeem] = useState(false);
  const [subDialog, setSubDialog] = useState(false);

  // connecting is gated by an active subscription — otherwise show the
  // "нужна подписка" dialog instead of talking to the tunnel
  const guardedConnect = (serverId: string) => {
    if (acct.hasSub) vpn.connect(serverId);
    else setSubDialog(true);
  };

  const openProfile = (redeem = false) => {
    setSubDialog(false);
    setFocusRedeem(redeem);
    setProfileOpen(true);
  };

  return (
    <div className="app">
      <Sidebar
        servers={vpn.servers}
        status={vpn.status}
        selectedId={vpn.selectedId}
        last={vpn.last}
        onSelect={vpn.select}
        onConnect={guardedConnect}
      />

      <div className="main">
        <StatusBar
          status={vpn.status}
          connectedServer={vpn.connectedServer}
          target={vpn.target}
          upRate={vpn.upRate}
          downRate={vpn.downRate}
          account={acct.account}
          hasSub={acct.hasSub}
          onConnect={guardedConnect}
          onDisconnect={vpn.disconnect}
          onOpenSettings={() => setSettingsOpen(true)}
          onOpenProfile={() => openProfile(false)}
        />

        <WorldMap
          servers={vpn.servers}
          status={vpn.status}
          selectedId={vpn.selectedId}
          onSelect={vpn.select}
          onConnect={guardedConnect}
          onDisconnect={vpn.disconnect}
        />

        {settingsOpen && <SettingsPanel onClose={() => setSettingsOpen(false)} />}

        {profileOpen && acct.account && (
          <ProfilePanel
            account={acct.account}
            focusRedeem={focusRedeem}
            onRedeem={acct.redeem}
            onLogout={acct.logout}
            onClose={() => setProfileOpen(false)}
          />
        )}

        {subDialog && (
          <SubscriptionDialog
            onRenew={() => openProfile(false)}
            onUseCode={() => openProfile(true)}
            onClose={() => setSubDialog(false)}
          />
        )}
      </div>
    </div>
  );
}
