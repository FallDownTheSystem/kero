import { Link } from './Link';
import { GitHubIcon } from './Icons';
import Search from './Search';

export const GITHUB_URL = 'https://github.com/egoist/kero';

export default function SiteHeader(props: { path: string }) {
  const inDocs = () => props.path.startsWith('/docs');
  return (
    <header class="site-header">
      <div class="wrapper site-header__inner">
        <Link href="/" class="logo" aria-label="Kero home">
          <img src="/icon.png" alt="" width={28} height={28} />
          <span>Kero.</span>
        </Link>
        <nav class="site-nav">
          <Link href="/docs" classList={{ 'is-active': inDocs() && props.path !== '/docs/cli' && props.path !== '/docs/agents' }}>
            Docs
          </Link>
          <Link href="/docs/cli" classList={{ 'is-active': props.path === '/docs/cli' }}>
            CLI
          </Link>
          <Link href="/docs/agents" class="hide-sm" classList={{ 'is-active': props.path === '/docs/agents' }}>
            Agents
          </Link>
          <Link href="/download" classList={{ 'is-active': props.path === '/download' }}>
            Download
          </Link>
        </nav>
        <div class="site-header__end">
          <Search />
          <a class="icon-link" href={GITHUB_URL} aria-label="Kero on GitHub">
            <GitHubIcon size={18} />
          </a>
        </div>
      </div>
    </header>
  );
}
