import { Link } from './Link';
import { GITHUB_URL } from './SiteHeader';

export default function SiteFooter() {
  return (
    <footer class="site-footer">
      <div class="wrapper site-footer__inner">
        <div class="site-footer__brand">
          <img src="/icon.png" alt="" width={22} height={22} />
          <span>Kero</span>
          <span class="muted">· a terminal for coding agents</span>
        </div>
        <nav>
          <Link href="/docs">Docs</Link>
          <Link href="/docs/configuration">Configuration</Link>
          <Link href="/docs/shortcuts">Shortcuts</Link>
          <a href={GITHUB_URL}>GitHub</a>
        </nav>
      </div>
    </footer>
  );
}
