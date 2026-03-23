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
      { title: 'Quick Start', href: '/docs/getting-started' },
      { title: 'Data Passing', href: '/docs/getting-started/data-passing' },
      { title: 'AI Workflows', href: '/docs/getting-started/ai-workflows' },
      { title: 'Production', href: '/docs/getting-started/production' },
    ],
  },
  {
    title: 'Concepts',
    items: [
      { title: 'Architecture', href: '/docs/concepts' },
      { title: 'Execution Model', href: '/docs/concepts/execution' },
      { title: 'CEL Expressions', href: '/docs/concepts/expressions' },
      { title: 'Security Model', href: '/docs/concepts/security' },
    ],
  },
  {
    title: 'Guides',
    items: [
      { title: 'Configuration', href: '/docs/configuration' },
      { title: 'Secrets & Credentials', href: '/docs/secrets-guide' },
      { title: 'Server Overview', href: '/docs/server-guide' },
      { title: 'Triggers', href: '/docs/server-guide/triggers' },
      { title: 'REST API', href: '/docs/server-guide/api' },
      { title: 'Operations', href: '/docs/server-guide/operations' },
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
      { title: 'Connectors', href: '/docs/workflow-reference/connectors' },
      { title: 'AI Tool Use', href: '/docs/workflow-reference/tools' },
      { title: 'CLI Overview', href: '/docs/cli-reference' },
      { title: 'Workflow Commands', href: '/docs/cli-reference/workflow-commands' },
      { title: 'Server Commands', href: '/docs/cli-reference/server-commands' },
      { title: 'Auth & Team Commands', href: '/docs/cli-reference/auth-commands' },
      { title: 'Admin Commands', href: '/docs/cli-reference/admin-commands' },
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
