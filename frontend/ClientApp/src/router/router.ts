import {
  ArrowsRightLeftIcon,
  CheckCircleIcon,
  CircleStackIcon,
  ClipboardDocumentListIcon,
  DocumentTextIcon,
  EyeIcon,
  PencilSquareIcon,
  SquaresPlusIcon,
} from '@heroicons/vue/20/solid'
import NewContractTemplateView from '@template-repository/views/NewContractTemplateView.vue'
import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'
import { getUIBasePath } from '@/config'
import { useScrollStore } from '@/core/store/scroll'
import { OID4VP_STATE_KEY } from '@/hydra-login-guard'
import SemanticHubView from '@/modules/semantic-hub/views/SemanticHubView.vue'
import TemplateCatalogueListView from '@/modules/template-catalogue/views/TemplateCatalogueListView.vue'
import TemplateCatalogueView from '@/modules/template-catalogue/views/TemplateCatalogueView.vue'
import ApproveContractTemplateView from '@/modules/template-repository/views/ApproveContractTemplateView.vue'
import ReviewContractTemplateView from '@/modules/template-repository/views/ReviewContractTemplateView.vue'
import ViewContractTemplateView from '@/modules/template-repository/views/ViewContractTemplateView.vue'
import { authenticationService } from '@/services/authentication-service'
import { useAuthStore } from '@/stores/auth-store'
import { useAuthTokenStore } from '@/stores/auth-token-store'
import { useNavStore } from '@/stores/nav-store'
import AuditView from '@/views/audit/AuditView.vue'
import AuthSuccessView from '@/views/auth/AuthSuccessView.vue'
import LoginView from '@/views/auth/LoginView.vue'
import PidPresentationView from '@/views/auth/PidPresentationView.vue'
import ApproveContractView from '@/views/contract/ApproveContractView.vue'
import ContractListView from '@/views/contract/ContractListView.vue'
import NegotiateContractView from '@/views/contract/NegotiateContractView.vue'
import NewContractView from '@/views/contract/NewContractView.vue'
import ReviewContractView from '@/views/contract/ReviewContractView.vue'
import ViewContractView from '@/views/contract/ViewContractView.vue'
import ContractTemplateListView from '@/views/contract-template-list/ContractTemplateListView.vue'
import FrontPageView from '@/views/FrontPageView.vue'
import SigningDashboardView from '@/views/signing/SigningDashboardView.vue'
import TaskListView from '@/views/task/TaskListView.vue'

const ROUTES = {
  HOME: 'home',
  FRONT_PAGE: 'front_page',
  TEMPLATES: {
    LIST: 'templates.list',
    NEW: 'templates.new',
    EDIT: 'templates.edit',
    VIEW: 'templates.view',
    REVIEW: 'templates.review',
    APPROVE: 'templates.approve',
  },
  TASKS: {
    REVIEWS: 'tasks.reviews',
    APPROVALS: 'tasks.approvals',
    NEGOTIATIONS: 'tasks.negotiations',
  },
  TEMPLATE_CATALOGUES: {
    LIST: 'template.catalogues.list',
    VIEW: 'template.catalogues.view',
  },
  AUDIT: {
    LIST: 'audit.list',
  },
  AUTH: {
    SUCCESS: 'auth.success',
    PID_VERIFY: 'auth.pid_verify',
  },
  CONTRACTS: {
    LIST: 'contracts.list',
    NEW: 'contracts.new',
    EDIT: 'contracts.edit',
    VIEW: 'contracts.view',
    NEGOTIATE: 'contracts.negotiate',
    REVIEW: 'contracts.review',
    APPROVE: 'contracts.approve',
  },
  SIGNING: {
    DASHBOARD: 'signing.dashboard',
  },
  SEMANTIC_HUB: {
    DASHBOARD: 'semantic_hub.dashboard',
  },
} as const

const routes: RouteRecordRaw[] = [
  {
    path: '/',
    name: ROUTES.HOME,
    meta: { name: 'DCS', hideInSidebar: true, requiresAuth: false, layout: 'blank', title: 'DCS' },
    component: LoginView,
  },
  {
    path: '/frontpage',
    name: ROUTES.FRONT_PAGE,
    component: FrontPageView,
    meta: {
      name: 'DCS',
      hideInSidebar: true,
      requiresAuth: true,
      title: 'DCS',
    },
  },
  {
    path: '/templates',
    name: ROUTES.TEMPLATES.LIST,
    component: ContractTemplateListView,
    meta: {
      name: 'Templates',
      icon: SquaresPlusIcon,
      requiresAuth: true,
      title: 'DCS - Templates',
      order: 1,
      roles: ['TEMPLATE_CREATOR', 'TEMPLATE_REVIEWER', 'TEMPLATE_APPROVER', 'TEMPLATE_MANAGER'],
    },
  },
  {
    path: '/templates/new',
    name: ROUTES.TEMPLATES.NEW,
    component: NewContractTemplateView,
    meta: {
      name: 'New Template',
      hideInSidebar: true,
      requiresAuth: true,
      title: 'DCS - New Template',
      roles: ['TEMPLATE_CREATOR', 'TEMPLATE_REVIEWER', 'TEMPLATE_APPROVER', 'TEMPLATE_MANAGER'],
    },
  },
  {
    path: '/templates/edit/:did',
    name: ROUTES.TEMPLATES.EDIT,
    component: NewContractTemplateView,
    meta: {
      name: 'Edit Template',
      hideInSidebar: true,
      requiresAuth: true,
      title: 'DCS - Edit Template',
      roles: ['TEMPLATE_CREATOR', 'TEMPLATE_REVIEWER', 'TEMPLATE_APPROVER', 'TEMPLATE_MANAGER'],
    },
  },
  {
    path: '/templates/view/:did',
    name: ROUTES.TEMPLATES.VIEW,
    component: ViewContractTemplateView,
    props: true,
    meta: {
      name: 'View Template',
      hideInSidebar: true,
      requiresAuth: true,
      title: 'DCS - View Template',
      roles: ['TEMPLATE_CREATOR', 'TEMPLATE_REVIEWER', 'TEMPLATE_APPROVER', 'TEMPLATE_MANAGER'],
    },
  },
  {
    path: '/templates/review/:did',
    name: ROUTES.TEMPLATES.REVIEW,
    component: ReviewContractTemplateView,
    meta: {
      name: 'Review Template',
      hideInSidebar: true,
      requiresAuth: true,
      title: 'DCS - Review Template',
      roles: ['TEMPLATE_CREATOR', 'TEMPLATE_REVIEWER', 'TEMPLATE_APPROVER', 'TEMPLATE_MANAGER'],
    },
  },
  {
    path: '/templates/approve/:did',
    name: ROUTES.TEMPLATES.APPROVE,
    component: ApproveContractTemplateView,
    meta: {
      name: 'Approve Template',
      hideInSidebar: true,
      requiresAuth: true,
      title: 'DCS - Approve Template',
      roles: ['TEMPLATE_CREATOR', 'TEMPLATE_REVIEWER', 'TEMPLATE_APPROVER', 'TEMPLATE_MANAGER'],
    },
  },
  {
    path: '/tasks/reviews',
    name: ROUTES.TASKS.REVIEWS,
    component: TaskListView,
    meta: {
      name: 'Review Tasks',
      icon: EyeIcon,
      requiresAuth: true,
      title: 'DCS - Review Tasks',
      order: 3.1,
      roles: ['TEMPLATE_REVIEWER', 'CONTRACT_REVIEWER'],
    },
  },
  {
    path: '/tasks/approvals',
    name: ROUTES.TASKS.APPROVALS,
    component: TaskListView,
    meta: {
      name: 'Approval Tasks',
      icon: CheckCircleIcon,
      requiresAuth: true,
      title: 'DCS - Approval Tasks',
      order: 3.2,
      roles: ['TEMPLATE_APPROVER', 'CONTRACT_APPROVER'],
    },
  },
  {
    path: '/tasks/negotiations',
    name: ROUTES.TASKS.NEGOTIATIONS,
    component: TaskListView,
    meta: {
      name: 'Negotiation Tasks',
      icon: ArrowsRightLeftIcon,
      requiresAuth: true,
      title: 'DCS - Negotiation Tasks',
      order: 3.3,
      roles: ['CONTRACT_CREATOR', 'CONTRACT_NEGOTIATOR', 'CONTRACT_REVIEWER'],
    },
  },
  {
    path: '/catalogues/templates',
    name: ROUTES.TEMPLATE_CATALOGUES.LIST,
    component: TemplateCatalogueListView,
    meta: {
      name: 'Template Catalogue',
      icon: DocumentTextIcon,
      requiresAuth: true,
      title: 'DCS - Template Catalogue',
      order: 4,
      roles: ['TEMPLATE_MANAGER'],
    },
  },
  {
    path: '/catalogues/templates/view/:did',
    name: ROUTES.TEMPLATE_CATALOGUES.VIEW,
    component: TemplateCatalogueView,
    props: true,
    meta: {
      name: 'Template Catalogue View',
      hideInSidebar: true,
      requiresAuth: true,
      title: 'DCS - Template Catalogue View',
      roles: ['TEMPLATE_MANAGER'],
    },
  },
  {
    path: '/audit',
    name: ROUTES.AUDIT.LIST,
    component: AuditView,
    meta: {
      name: 'Audit',
      icon: ClipboardDocumentListIcon,
      requiresAuth: true,
      title: 'DCS - Audit',
      order: 5,
      roles: ['AUDITOR', 'ARCHIVE_MANAGER'],
    },
  },
  {
    path: '/contracts',
    name: ROUTES.CONTRACTS.LIST,
    component: ContractListView,
    meta: {
      name: 'Contracts',
      icon: DocumentTextIcon,
      requiresAuth: true,
      title: 'DCS - Contracts',
      order: 2,
      roles: [
        'CONTRACT_CREATOR',
        'CONTRACT_NEGOTIATOR',
        'CONTRACT_REVIEWER',
        'CONTRACT_APPROVER',
        'CONTRACT_MANAGER',
        'CONTRACT_OBSERVER',
      ],
    },
  },
  {
    path: '/contracts/new',
    name: ROUTES.CONTRACTS.NEW,
    component: NewContractView,
    meta: {
      name: 'New Contract',
      hideInSidebar: true,
      requiresAuth: true,
      title: 'DCS - New Contract',
      roles: ['CONTRACT_CREATOR'],
    },
  },
  {
    path: '/contracts/edit/:did',
    name: ROUTES.CONTRACTS.EDIT,
    component: NewContractView,
    meta: {
      name: 'Edit Contract',
      hideInSidebar: true,
      requiresAuth: true,
      title: 'DCS - Edit Contract',
      roles: ['CONTRACT_CREATOR'],
    },
  },
  {
    path: '/contracts/view/:did',
    name: ROUTES.CONTRACTS.VIEW,
    component: ViewContractView,
    meta: {
      name: 'View Contract',
      hideInSidebar: true,
      requiresAuth: true,
      title: 'DCS - View Contract',
      roles: [
        'CONTRACT_CREATOR',
        'CONTRACT_NEGOTIATOR',
        'CONTRACT_REVIEWER',
        'CONTRACT_APPROVER',
        'CONTRACT_MANAGER',
        'CONTRACT_OBSERVER',
      ],
    },
  },
  {
    path: '/contracts/negotiate/:did',
    name: ROUTES.CONTRACTS.NEGOTIATE,
    component: NegotiateContractView,
    meta: {
      name: 'Negotiate Contract',
      hideInSidebar: true,
      requiresAuth: true,
      title: 'DCS - Negotiate Contract',
      roles: [
        'CONTRACT_CREATOR',
        'CONTRACT_NEGOTIATOR',
        'CONTRACT_REVIEWER',
        'CONTRACT_APPROVER',
        'CONTRACT_MANAGER',
        'CONTRACT_OBSERVER',
      ],
    },
  },
  {
    path: '/contracts/review/:did',
    name: ROUTES.CONTRACTS.REVIEW,
    component: ReviewContractView,
    meta: {
      name: 'Review Contract',
      hideInSidebar: true,
      requiresAuth: true,
      title: 'DCS - Review Contract',
      roles: [
        'CONTRACT_CREATOR',
        'CONTRACT_NEGOTIATOR',
        'CONTRACT_REVIEWER',
        'CONTRACT_APPROVER',
        'CONTRACT_MANAGER',
        'CONTRACT_OBSERVER',
      ],
    },
  },
  {
    path: '/contracts/approve/:did',
    name: ROUTES.CONTRACTS.APPROVE,
    component: ApproveContractView,
    meta: {
      name: 'Approve Contract',
      hideInSidebar: true,
      requiresAuth: true,
      title: 'DCS - Approve Contract',
      roles: [
        'CONTRACT_CREATOR',
        'CONTRACT_NEGOTIATOR',
        'CONTRACT_REVIEWER',
        'CONTRACT_APPROVER',
        'CONTRACT_MANAGER',
        'CONTRACT_OBSERVER',
      ],
    },
  },
  {
    path: '/signing',
    name: ROUTES.SIGNING.DASHBOARD,
    component: SigningDashboardView,
    meta: {
      name: 'Signing Dashboard',
      icon: PencilSquareIcon,
      requiresAuth: true,
      title: 'DCS - Signing Dashboard',
      order: 5,
      roles: ['CONTRACT_SIGNER', 'CONTRACT_MANAGER', 'CONTRACT_OBSERVER'],
    },
  },
  {
    path: '/semantic-hub',
    name: ROUTES.SEMANTIC_HUB.DASHBOARD,
    component: SemanticHubView,
    meta: {
      name: 'Semantic Hub',
      icon: CircleStackIcon,
      requiresAuth: true,
      title: 'DCS - Semantic Hub',
      order: 6,
      roles: ['TEMPLATE_MANAGER'],
    },
  },
  {
    path: '/auth/success',
    name: ROUTES.AUTH.SUCCESS,
    meta: { hideInSidebar: true, requiresAuth: false, layout: 'blank', title: 'DCS - Auth Success' },
    component: AuthSuccessView,
  },
  {
    path: '/pid-verify',
    name: ROUTES.AUTH.PID_VERIFY,
    meta: { hideInSidebar: true, requiresAuth: false, layout: 'blank', title: 'DCS - PID Verify' },
    component: PidPresentationView,
  },
]

const router = createRouter({
  history: createWebHistory(getUIBasePath()),
  routes: routes,
})

router.beforeEach(async (to) => {
  const authStore = useAuthStore()

  // Refresh when localStorage has tokens and OID4VP state is absent. Redirect if authenticated.
  if (to.name === ROUTES.HOME) {
    if (!authStore.isAuthenticated) {
      const hadAccessToken = useAuthTokenStore().isAuthSet
      const oid4vpLoginActive = !!sessionStorage.getItem(OID4VP_STATE_KEY)
      if (hadAccessToken && !oid4vpLoginActive) {
        await authenticationService.refresh()
      }
    }
    if (authStore.isAuthenticated) {
      return { name: ROUTES.FRONT_PAGE }
    }
    return true
  }

  if (to.meta.requiresAuth === false) {
    return true
  }

  if (authStore.isAuthenticated) {
    return true
  }

  // A valid stored token already carries the identity — restore it without a
  // refresh round-trip; only refresh when there is no usable token (its
  // rotating refresh cookie is single-use, so it must not be spent on every
  // navigation).
  if (authStore.restoreFromToken()) {
    return true
  }

  await authenticationService.refresh()
  if (authStore.isAuthenticated) {
    return true
  }

  return { name: ROUTES.HOME }
})

router.beforeEach((to) => {
  if (!to.meta.roles) {
    return true
  }
  const authStore = useAuthStore()
  const hasAuthorizedRole = authStore.user?.roles?.some((role) => to.meta.roles?.includes(role)) ?? false
  if (!hasAuthorizedRole) {
    return { name: ROUTES.HOME }
  }
})

router.beforeEach((to) => {
  const scrollStore = useScrollStore()
  if (to.matched.some((r) => r.path.includes(':'))) {
    scrollStore.addGutter()
  } else {
    scrollStore.removeGutter()
  }
})

router.beforeEach((_, from) => {
  const navStore = useNavStore()
  navStore.previousRoute = from
})

export { router, ROUTES }
