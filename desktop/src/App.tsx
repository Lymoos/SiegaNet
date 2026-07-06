import { useState } from "react";
import { useVpn } from "./state/useVpn";
import { StatusBar } from "./components/StatusBar";
import { Sidebar } from "./components/Sidebar";
import { WorldMap } from "./components/WorldMap";
import { SettingsPanel } from "./components/SettingsPanel";
import { ActivationScreen } from "./components/ActivationScreen";
import { isActivated } from "./state/activation";

export default function App() {
  const [activated, setActivated] = useState(isActivated);

  if (!activated) {
    return <ActivationScreen onActivated={() => setActivated(true)} />;
  }
  return <MainApp />;
}

function MainApp() {
  const vpn = useVpn();
  const [settingsOpen, setSettingsOpen] = useState(false);

  return (
    <div className="app">
      <Sidebar
        servers={vpn.servers}
        status={vpn.status}
        selectedId={vpn.selectedId}
        last={vpn.last}
        onSelect={vpn.select}
        onConnect={vpn.connect}
      />

      <div className="main">
        <StatusBar
          status={vpn.status}
          connectedServer={vpn.connectedServer}
          target={vpn.target}
          upRate={vpn.upRate}
          downRate={vpn.downRate}
          onConnect={vpn.connect}
          onDisconnect={vpn.disconnect}
          onOpenSettings={() => setSettingsOpen(true)}
        />

        <WorldMap
          servers={vpn.servers}
          status={vpn.status}
          selectedId={vpn.selectedId}
          onSelect={vpn.select}
          onConnect={vpn.connect}
          onDisconnect={vpn.disconnect}
        />

        {settingsOpen && <SettingsPanel onClose={() => setSettingsOpen(false)} />}
      </div>
    </div>
  );
}
