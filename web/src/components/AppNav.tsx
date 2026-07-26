import type { MouseEvent } from "react";

interface AppNavProps {
  pathname: string;
  onNavigate: (path: string) => void;
}

export function AppNav({ pathname, onNavigate }: AppNavProps) {
  const navigate = (event: MouseEvent<HTMLAnchorElement>, path: string) => {
    event.preventDefault();
    onNavigate(path);
  };

  return (
    <header className="app-header">
      <a
        className="brand"
        href="/"
        aria-label="TinyProbe 首页"
        onClick={(event) => navigate(event, "/")}
      >
        <span className="brand-mark" aria-hidden="true">
          T
        </span>
        <span translate="no">TinyProbe</span>
      </a>
      <nav aria-label="主导航">
        <a
          className={`nav-link${pathname === "/" ? " nav-link-current" : ""}`}
          href="/"
          aria-current={pathname === "/" ? "page" : undefined}
          onClick={(event) => navigate(event, "/")}
        >
          概览
        </a>
      </nav>
    </header>
  );
}
