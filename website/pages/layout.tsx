import '@fontsource-variable/inter';
import '@fontsource-variable/geist';
import '@fontsource-variable/geist-mono';
import '@fontsource-variable/jetbrains-mono';
import '@void/md/theme-content.css';
import '../styles/global.css';
import type { JSX } from 'solid-js';
import { useRouter } from '@void/solid';
import SiteHeader from '../components/SiteHeader';
import SiteFooter from '../components/SiteFooter';

export default function Layout(props: { children?: JSX.Element }) {
  const router = useRouter();
  const path = () => router.path.replace(/\/$/, '') || '/';
  return (
    <div class="site" classList={{ 'site--home': path() === '/' }}>
      <SiteHeader path={path()} />
      {props.children}
      <SiteFooter />
    </div>
  );
}
