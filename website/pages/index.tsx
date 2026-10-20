import { For } from 'solid-js';
import { Dynamic } from 'solid-js/web';
import { Link } from '@void/solid';
import AppWindow from '../components/AppWindow';
import CliShowcase from '../components/CliShowcase';
import ParticleMorph from '../components/ParticleMorph';
import { ArrowUpRightIcon, BellIcon, BotIcon, LayersIcon, TerminalIcon } from '../components/Icons';

const features = [
  {
    icon: LayersIcon,
    title: 'Sessions That Outlive the Window',
    body: 'Sessions run in kerod, a daemon of their own. Quit Kero and they keep going; open it again and everything is where you left it.',
    points: ['Scrollback survives quitting', 'Sessions start again after a reboot', 'Grouped by project in the sidebar'],
    link: { href: '/docs/sessions', label: 'Sessions & projects' },
  },
  {
    icon: BellIcon,
    title: 'Know Which Ones Need You',
    body: 'Every pane is working, waiting, idle, done or failed. The ones waiting on you stand out, and one shortcut takes you there.',
    points: ['⌘J jumps to the session needing you', 'Notifies you when an agent finishes', 'Dock badge counts who is waiting'],
    link: { href: '/docs/status', label: 'Status & notifications' },
  },
  {
    icon: BotIcon,
    title: 'A CLI Your Agents Can Drive',
    body: 'Agents start dev servers beside themselves, hand work to other agents, wait for tests and report their own progress.',
    points: ['kero split, send, wait and read', 'One command installs the agent skill', 'Works with Claude Code, Codex and more'],
    link: { href: '/docs/agents', label: 'Coding agents' },
  },
  {
    icon: TerminalIcon,
    title: 'Ghostty Under the Hood',
    body: 'Terminals render with libghostty: fast, correct and familiar. Ghostty themes and configuration keys work as you expect.',
    points: ['Every Ghostty theme, light and dark', 'Plain-text config with live reload', 'Rebind any action'],
    link: { href: '/docs/configuration', label: 'Configuration' },
  },
];

export default function HomePage() {
  return (
    <main class="home">
      {/* Hero */}
      <section class="wrapper hero">
        <div class="hero__copy">
          <p class="eyebrow eyebrow--brand">
            <span>Kero</span>
            <span class="eyebrow__sep" />
            <span>for coding agents</span>
          </p>
          <h1 class="display">
            Run your agents.
            <br />
            Keep your place.
          </h1>
          <p class="lead">
            Kero is a terminal for coding agents. Sessions keep running when you quit, and the sidebar shows
            which ones are working, which need you, and which are done.
          </p>
          <div class="actions">
            <Link href="/docs/quick-start" class="button button--primary">
              Get Started
            </Link>
            <Link href="/download" class="button">
              Download
            </Link>
          </div>
        </div>
        <div class="hero__art">
          <div class="hero__glow" aria-hidden="true" />
          <ParticleMorph />
        </div>
      </section>

      {/* The app */}
      <section class="wrapper section-head">
        <h2 class="title">Every Agent, One Window</h2>
        <p class="lead lead--center">
          An agent reviews your change, lazygit shows the diff, and the program runs beside them. The sidebar
          tells you which session needs you.
        </p>
      </section>
      <section class="wrapper showcase">
        <div class="stage">
          <div class="glass">
            <div class="stage__frame">
              <AppWindow />
            </div>
          </div>
        </div>
      </section>

      <section class="wrapper ticks" aria-hidden="true" />

      {/* CLI */}
      <section class="wrapper section-head">
        <h2 class="title">Drive It From the Shell</h2>
        <p class="lead lead--center">
          Start sessions, split panes, type into them, wait for them and read their screens, from your shell or
          your agent's.
        </p>
      </section>
      <section class="wrapper wrapper--flush">
        <CliShowcase />
      </section>

      <section class="wrapper ticks" aria-hidden="true" />

      {/* Features, on light */}
      <div class="light">
        <section class="wrapper section-head">
          <h2 class="title">Built for Many Agents at Once</h2>
          <p class="lead lead--center">
            One window for every task in flight, and a clear answer to the only question that matters: who needs
            you right now?
          </p>
        </section>
        <section class="wrapper features">
          <For each={features}>
            {(f) => (
            <article class="feature">
              <Dynamic component={f.icon} size={20} class="feature__icon" />
              <h3>{f.title}</h3>
              <p>{f.body}</p>
              <ul class="checklist">
                <For each={f.points}>
                  {(p) => (
                    <li>
                      <span class="checklist__mark">[✓]</span>
                      {p}
                    </li>
                  )}
                </For>
              </ul>
              <Link href={f.link.href} class="pill-link">
                {f.link.label} <ArrowUpRightIcon size={14} />
              </Link>
            </article>
            )}
          </For>
        </section>
        <section class="wrapper ticks" aria-hidden="true" />
      </div>

      {/* CTA */}
      <section class="wrapper cta">
        <img src="/icon.png" alt="" width={88} height={88} class="cta__icon" />
        <h2 class="title">Hop In</h2>
        <p class="lead lead--center">Install Kero, teach your agents to use it, and stop losing track of them.</p>
        <pre class="cta__code">
          <span class="t-dim"># after installing the command line tool</span>
          {'\n'}
          <span class="t-accent">$</span> kero skill setup{'\n'}
          <span class="t-accent">$</span> kero new -n my-task -- claude
        </pre>
        <div class="actions actions--center">
          <Link href="/docs/quick-start" class="button button--primary">
            Get Started
          </Link>
          <Link href="/docs/cli" class="button">
            CLI Reference
          </Link>
        </div>
      </section>
    </main>
  );
}
