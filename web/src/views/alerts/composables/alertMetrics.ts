import type { SemanticAlert } from './alertSemantics'

export interface MetricDefinition {
  title: string
  meaning: string
  suggestion: string
}

const METRICS: Record<string, Record<string, MetricDefinition>> = {
  'SYS.RDS': {
    oldest_transaction_duration: {
      title: '最长事务存活时长',
      meaning: '统计当前数据库中存在的最长事务存活时长。长时间未结束的事务可能带来锁等待和连接占用。',
      suggestion: '建议检查未提交事务、锁等待和长时间占用连接的 SQL。'
    },
    cpu_util: {
      title: 'CPU 使用率',
      meaning: '统计数据库实例的 CPU 使用率，持续过高可能影响 SQL 响应和连接处理。',
      suggestion: '建议结合慢 SQL、活跃连接和实例规格继续排查。'
    },
    mem_util: {
      title: '内存使用率',
      meaning: '统计数据库实例的内存使用率，持续过高可能影响数据库稳定性。',
      suggestion: '建议结合缓存配置、连接数和慢 SQL 继续排查。'
    }
  },
  'SYS.ECS': {
    cpu_util: {
      title: 'CPU 使用率',
      meaning: '统计云服务器的 CPU 使用率，持续过高可能导致应用响应变慢。',
      suggestion: '建议结合进程负载、实例规格和同时段业务流量继续排查。'
    },
    mem_util: {
      title: '内存使用率',
      meaning: '统计云服务器的内存使用率，持续过高可能触发应用性能下降或内存回收。',
      suggestion: '建议检查高内存进程、缓存增长和实例规格。'
    },
    disk_util: {
      title: '磁盘使用率',
      meaning: '统计云服务器磁盘空间的使用比例，空间不足可能影响日志、数据和系统写入。',
      suggestion: '建议检查大文件、日志轮转、临时文件和数据增长趋势。'
    },
    load_average1: {
      title: '1 分钟平均负载',
      meaning: '统计云服务器最近 1 分钟的平均负载，用于观察短期资源竞争。',
      suggestion: '建议结合 CPU 核数、运行队列和主要进程负载继续判断。'
    }
  }
}

const NAMESPACES: Record<string, string> = {
  'SYS.RDS': '云数据库 RDS',
  'SYS.ECS': '云服务器 ECS',
  'SYS.EVS': '云硬盘 EVS',
  'SYS.VPC': '虚拟私有云 VPC',
  'SYS.ELB': '负载均衡 ELB',
  'SYS.DCS': '分布式缓存 DCS',
  'SYS.DMS': '分布式消息 DMS'
}

export function metricDefinition(namespace: string, metricName: string): MetricDefinition | undefined {
  return METRICS[namespace]?.[metricName]
}

export function namespaceText(namespace: string) {
  return NAMESPACES[namespace] || namespace || '未知命名空间'
}

export function resourceKindText(alert: SemanticAlert) {
  const labels = alert.labels || {}
  if (labels.postgresql_cluster_id || labels.rds_cluster_id) return 'RDS PostgreSQL 集群'
  const type = labels.resource_type || alert.resource_type || ''
  const kinds: Record<string, string> = {
    rds: 'RDS 数据库实例',
    ecs: '云服务器',
    evs: '云硬盘',
    elb: '负载均衡器',
    vpc: 'VPC 网络',
    subnet: 'VPC 子网',
    dcs: '分布式缓存实例',
    dms: '分布式消息实例',
    pod: 'Kubernetes Pod',
    host: '主机'
  }
  return kinds[type] || type || '云资源'
}
