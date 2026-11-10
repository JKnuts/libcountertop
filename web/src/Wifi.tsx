import { useCallback, useEffect, useRef, useState } from "react";
import { Alert, Badge, Button, Group, Paper, PasswordInput, Progress, Stack, Text, UnstyledButton } from "@mantine/core";
import { IconLock, IconWifi } from "@tabler/icons-react";
import { api, type WifiNetwork, type WifiState } from "./api";

const pollEvery = 1000;
const windowSeconds = 120;

type Phase =
  | { kind: "idle" }
  | { kind: "moving"; ssid: string; deadline: number; lost: boolean }
  | { kind: "done"; ssid: string }
  | { kind: "failed"; ssid: string; why: string };

function deadlineOf(s: WifiState): number {
  if (!s.move) return Date.now();
  return Date.now() + (Date.parse(s.move.until) - Date.parse(s.now));
}

function bars(signal: number): string {
  if (signal >= -55) return "strong";
  if (signal >= -70) return "good";
  return "weak";
}

export function Wifi({ name }: { name: string }) {
  const [network, setNetwork] = useState("");
  const [phase, setPhase] = useState<Phase>({ kind: "idle" });
  const [found, setFound] = useState<WifiNetwork[]>();
  const [scanning, setScanning] = useState(false);
  const [chosen, setChosen] = useState<WifiNetwork>();
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [now, setNow] = useState(Date.now());
  const timer = useRef<number | undefined>(undefined);

  const apply = useCallback((s: WifiState) => {
    setNetwork(s.network);
    const m = s.move;
    if (!m) return;
    if (m.moving) setPhase({ kind: "moving", ssid: m.ssid, deadline: deadlineOf(s), lost: false });
    else if (m.failed) setPhase((p) => (p.kind === "moving" ? { kind: "failed", ssid: m.ssid, why: m.failed ?? "" } : p));
    else setPhase((p) => (p.kind === "moving" ? { kind: "done", ssid: m.ssid } : p));
  }, []);

  useEffect(() => {
    api.wifi().then(apply).catch((e: Error) => setError(e.message));
  }, [apply]);

  const moving = phase.kind === "moving";
  useEffect(() => {
    if (!moving) return;
    let stopped = false;
    const poll = () => {
      api
        .wifi(AbortSignal.timeout(3000))
        .then((s) => !stopped && apply(s))
        .catch(() => !stopped && setPhase((p) => (p.kind === "moving" ? { ...p, lost: true } : p)))
        .finally(() => {
          if (!stopped) timer.current = window.setTimeout(poll, pollEvery);
        });
    };
    poll();
    const tick = window.setInterval(() => setNow(Date.now()), 250);
    return () => {
      stopped = true;
      window.clearTimeout(timer.current);
      window.clearInterval(tick);
    };
  }, [moving, apply]);

  const scan = async () => {
    setScanning(true);
    setError("");
    try {
      setFound(await api.wifiNetworks());
    } catch (e) {
      setError(e instanceof Error ? e.message : "Try again");
    } finally {
      setScanning(false);
    }
  };

  const join = async () => {
    if (!chosen) return;
    setError("");
    try {
      await api.wifiMove(chosen.ssid, password);
      setPassword("");
      setPhase({ kind: "moving", ssid: chosen.ssid, deadline: Date.now() + windowSeconds * 1000, lost: false });
      setChosen(undefined);
      setFound(undefined);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Try again");
    }
  };

  const left = moving ? Math.max(0, Math.ceil((phase.deadline - now) / 1000)) : 0;

  return (
    <Paper withBorder p="md">
      <Stack gap="sm">
        <Group justify="space-between">
          <Group gap="xs">
            <IconWifi size={18} stroke={1.6} />
            <Text fw={600}>Wi-Fi</Text>
          </Group>
          {network && <Badge variant="light">{network}</Badge>}
        </Group>

        {phase.kind === "moving" && (
          <Stack gap={6}>
            <Text size="sm">
              Joining {phase.ssid}… {left}s
            </Text>
            <Progress value={(left / windowSeconds) * 100} size="sm" animated />
            {phase.lost && (
              <Text size="sm" c="dimmed">
                {name} left this network. If it joined {phase.ssid}, find it there. If it can't, it comes back here when the time runs out.
              </Text>
            )}
          </Stack>
        )}
        {phase.kind === "done" && <Alert color="green">Connected to {phase.ssid}.</Alert>}
        {phase.kind === "failed" && (
          <Alert color="red" title={`Couldn't join ${phase.ssid}`}>
            {phase.why}
          </Alert>
        )}

        {!moving && !found && (
          <Group>
            <Button variant="light" loading={scanning} onClick={() => void scan()}>
              Change network
            </Button>
          </Group>
        )}

        {!moving && found && (
          <Stack gap={4}>
            {found.length === 0 && <Text size="sm">No networks found.</Text>}
            {found.map((n) => (
              <UnstyledButton
                key={n.ssid}
                className="ct-row"
                data-focus={chosen?.ssid === n.ssid || undefined}
                disabled={!n.supported || n.ssid === network}
                onClick={() => {
                  setChosen(n);
                  setPassword("");
                }}
              >
                <Group justify="space-between" px="xs" py={6}>
                  <Group gap={6}>
                    {n.security !== "open" && <IconLock size={14} stroke={1.6} />}
                    <Text size="sm" c={!n.supported || n.ssid === network ? "dimmed" : undefined}>
                      {n.ssid}
                    </Text>
                  </Group>
                  <Text size="xs" c="dimmed">
                    {n.ssid === network ? "connected" : !n.supported ? `${n.security}, not supported` : bars(n.signal)}
                  </Text>
                </Group>
              </UnstyledButton>
            ))}
            {chosen && (
              <Stack gap="xs" mt="xs">
                {chosen.security !== "open" && (
                  <PasswordInput
                    label={`Password for ${chosen.ssid}`}
                    value={password}
                    onChange={(e) => setPassword(e.currentTarget.value)}
                    autoComplete="off"
                  />
                )}
                <Text size="xs" c="dimmed">
                  {name} keeps {network || "its current network"} until it gets an address on {chosen.ssid}, and goes back if it can't within {windowSeconds / 60} minutes.
                </Text>
                <Group>
                  <Button onClick={() => void join()} disabled={chosen.security !== "open" && password.length < 8}>
                    Switch
                  </Button>
                  <Button variant="subtle" onClick={() => setChosen(undefined)}>
                    Cancel
                  </Button>
                </Group>
              </Stack>
            )}
            <Group mt="xs">
              <Button size="xs" variant="subtle" loading={scanning} onClick={() => void scan()}>
                Scan again
              </Button>
              <Button size="xs" variant="subtle" onClick={() => setFound(undefined)}>
                Close
              </Button>
            </Group>
          </Stack>
        )}

        {error && (
          <Text size="sm" c="red">
            {error}
          </Text>
        )}
      </Stack>
    </Paper>
  );
}
