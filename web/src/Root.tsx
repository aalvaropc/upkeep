import { App } from "./App";
import { Landing } from "./components/Landing";

export function isDemoPath(pathname: string): boolean {
  return pathname.startsWith("/demo");
}

interface Props {
  pathname?: string;
}

export function Root({ pathname = window.location.pathname }: Props) {
  return isDemoPath(pathname) ? <App /> : <Landing />;
}
