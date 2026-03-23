export interface NavItem {
  title: string;
  href: string;
}

export interface NavGroup {
  title: string;
  items: NavItem[];
}

export const docsNav: NavGroup[] = [
  {
    title: 'Getting Started',
    items: [
      { title: 'Introduction', href: '/docs/getting-started' },
      { title: 'Concepts', href: '/docs/concepts' },
      { title: 'Configuration', href: '/docs/configuration' },
    ],
  },
  {
    title: 'Guides',
    items: [
      { title: 'Secrets & Credentials', href: '/docs/secrets-guide' },
      { title: 'Server & Triggers', href: '/docs/server-guide' },
      { title: 'Authentication & RBAC', href: '/docs/authentication-guide' },
      { title: 'Deployment', href: '/docs/deployment-guide' },
      { title: 'Plugins', href: '/docs/plugins-guide' },
      { title: 'Observability', href: '/docs/observability-guide' },
    ],
  },
  {
    title: 'Reference',
    items: [
      { title: 'Workflow YAML', href: '/docs/workflow-reference' },
      { title: 'CLI Commands', href: '/docs/cli-reference' },
    ],
  },
  {
    title: 'Resources',
    items: [
      { title: 'Comparison', href: '/docs/comparison' },
    ],
  },
];

/** Flat list of all nav items in order */
export function flatNavItems(): NavItem[] {
  return docsNav.flatMap((group) => group.items);
}

/** Get prev/next items for a given href */
export function getPrevNext(href: string): { prev: NavItem | null; next: NavItem | null } {
  const items = flatNavItems();
  const idx = items.findIndex((item) => item.href === href);
  return {
    prev: idx > 0 ? items[idx - 1] : null,
    next: idx < items.length - 1 ? items[idx + 1] : null,
  };
}
