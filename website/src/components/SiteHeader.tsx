import { Link } from './Link';
import { GitHubIcon } from './Icons';
import Search from './Search';
import { delocalize, localeName, localize, t, type Locale } from '../lib/i18n';

export const GITHUB_URL = 'https://github.com/egoist/kero';

export default function SiteHeader(props: { path: string; locale: Locale }) {
  const s = () => t(props.locale);
  // The page's own path, without the locale's prefix.
  const page = () => delocalize(props.path);
  const inDocs = () => page().startsWith('/docs');
  const other = (): Locale => (props.locale === 'en' ? 'zh' : 'en');
  const to = (path: string) => localize(path, props.locale);
  return (
    <header class="site-header">
      <div class="wrapper site-header__inner">
        <Link href={to('/')} class="logo" aria-label={s().home}>
          <img src="/icon.png" alt="" width={28} height={28} />
          <span>Kero.</span>
        </Link>
        <nav class="site-nav">
          <Link href={to('/docs')} classList={{ 'is-active': inDocs() && page() !== '/docs/cli' && page() !== '/docs/agents' }}>
            {s().docs}
          </Link>
          <Link href={to('/docs/cli')} classList={{ 'is-active': page() === '/docs/cli' }}>
            {s().cli}
          </Link>
          <Link href={to('/docs/agents')} class="hide-sm" classList={{ 'is-active': page() === '/docs/agents' }}>
            {s().agents}
          </Link>
          <Link href={to('/download')} classList={{ 'is-active': page() === '/download' }}>
            {s().download}
          </Link>
        </nav>
        <div class="site-header__end">
          <Search />
          {/* The same page in the other language. */}
          <Link href={localize(page(), other())} class="lang-link" hreflang={other() === 'zh' ? 'zh-CN' : 'en'} aria-label={s().language}>
            {localeName[other()]}
          </Link>
          <a class="icon-link" href={GITHUB_URL} aria-label={s().github}>
            <GitHubIcon size={18} />
          </a>
        </div>
      </div>
    </header>
  );
}
