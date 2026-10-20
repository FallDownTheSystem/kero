import { For, Show, createSignal, onMount } from 'solid-js';
import { Dynamic } from 'solid-js/web';
import { Link } from '@void/solid';
import { AppleIcon, ArrowUpRightIcon, DownloadIcon, LinuxIcon, WindowsIcon } from '../components/Icons';
import { GITHUB_URL } from '../components/SiteHeader';
import type { Props } from './download.server';

const INSTALL_SCRIPT = `curl -fsSL ${GITHUB_URL}/releases/latest/download/install.sh | sh`;

type OS = 'mac' | 'windows' | 'linux';
type Arch = 'arm' | 'x86';

// The release's assets, named as GitHub names them: the spaces in mygo
// build's names become dots.
const platforms: {
  os: OS;
  name: string;
  icon: typeof AppleIcon;
  note: string;
  files: { label: string; kind: string; arch: Arch; asset: (v: string) => string }[];
}[] = [
  {
    os: 'mac',
    name: 'macOS',
    icon: AppleIcon,
    note: 'Open the disk image and drag Kero into your Applications folder.',
    files: [
      { label: 'Apple Silicon', kind: '.dmg', arch: 'arm', asset: (v) => `Kero.${v}.arm64.dmg` },
      { label: 'Intel', kind: '.dmg', arch: 'x86', asset: (v) => `Kero.${v}.amd64.dmg` },
    ],
  },
  {
    os: 'windows',
    name: 'Windows',
    icon: WindowsIcon,
    note: 'Run the installer, which needs no administrator rights. Kero needs Windows 10 1809 or later.',
    files: [
      { label: 'x64', kind: '.exe', arch: 'x86', asset: (v) => `Kero.Setup.${v}.amd64.exe` },
      { label: 'ARM64', kind: '.exe', arch: 'arm', asset: (v) => `Kero.Setup.${v}.arm64.exe` },
    ],
  },
  {
    os: 'linux',
    name: 'Linux',
    icon: LinuxIcon,
    note: 'Install the package, or run the script to install Kero in ~/.local without root.',
    files: [
      { label: 'x64', kind: '.deb', arch: 'x86', asset: (v) => `kero_${v}_amd64.deb` },
      { label: 'ARM64', kind: '.deb', arch: 'arm', asset: (v) => `kero_${v}_arm64.deb` },
    ],
  },
];

type UAData = { platform?: string; getHighEntropyValues?: (hints: string[]) => Promise<{ architecture?: string }> };

function detectOS(): OS | undefined {
  const ua = navigator.userAgent;
  const platform = (navigator as { userAgentData?: UAData }).userAgentData?.platform || navigator.platform || ua;
  if (/mac/i.test(platform)) return 'mac';
  if (/win/i.test(platform)) return 'windows';
  if (/linux|x11/i.test(platform) && !/android/i.test(ua)) return 'linux';
}

// Only Chromium tells the architecture; elsewhere the first file is the pick.
async function detectArch(): Promise<Arch | undefined> {
  const data = (navigator as { userAgentData?: UAData }).userAgentData;
  const arch = await data?.getHighEntropyValues?.(['architecture']).then((v) => v.architecture, () => undefined);
  if (arch === 'arm') return 'arm';
  if (arch === 'x86') return 'x86';
}

export default function DownloadPage(props: Props) {
  const [os, setOS] = createSignal<OS>();
  const [arch, setArch] = createSignal<Arch>();

  onMount(() => {
    setOS(detectOS());
    detectArch().then(setArch);
  });

  const tag = () => `v${props.version}`;
  const href = (asset: (v: string) => string) =>
    `${GITHUB_URL}/releases/download/${tag()}/${asset(props.version)}`;
  const isPick = (p: (typeof platforms)[number], f: (typeof platforms)[number]['files'][number]) =>
    p.os === os() && (arch() ? f.arch === arch() : f === p.files[0]);

  return (
    <main class="download">
      <section class="wrapper download-head">
        <img src="/icon.png" alt="" width={88} height={88} class="cta__icon" />
        <h1 class="display">Download Kero</h1>
        <p class="lead lead--center">For macOS, Windows and Linux.</p>
        <p class="download-head__version">
          <a href={`${GITHUB_URL}/releases/tag/${tag()}`}>
            {tag()} release notes <ArrowUpRightIcon size={13} />
          </a>
        </p>
      </section>

      <section class="wrapper platforms">
        <For each={platforms}>
          {(p) => (
            <article class="platform" classList={{ 'is-yours': p.os === os() }}>
              <div class="platform__head">
                <Dynamic component={p.icon} size={26} class="platform__icon" />
                <h2>{p.name}</h2>
                <Show when={p.os === os()}>
                  <span class="platform__badge">Your system</span>
                </Show>
              </div>
              <p>{p.note}</p>
              <div class="platform__files">
                <For each={p.files}>
                  {(f) => (
                    <a href={href(f.asset)} class="button" classList={{ 'button--primary': isPick(p, f) }}>
                      <DownloadIcon size={16} />
                      {f.label}
                      <span class="platform__kind">{f.kind}</span>
                    </a>
                  )}
                </For>
              </div>
              <Show when={p.os === 'linux'}>
                <pre class="platform__script">
                  <span class="t-accent">$</span> {INSTALL_SCRIPT}
                </pre>
              </Show>
            </article>
          )}
        </For>
      </section>

      <section class="wrapper cta">
        <h2 class="title">Then, Meet the CLI</h2>
        <p class="lead lead--center">
          Choose <strong>Kero › Install Command Line Tool…</strong> to put <code>kero</code> on your PATH, and let
          your agents drive it.
        </p>
        <div class="actions actions--center">
          <Link href="/docs/quick-start" class="button button--primary">
            Quick Start
          </Link>
          <a href={`${GITHUB_URL}/releases`} class="button">
            All Releases
          </a>
        </div>
      </section>
    </main>
  );
}
