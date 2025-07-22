// 网络连接测试脚本
// 用于测试从Windows主机到WSL中服务的连接

const testUrls = [
  'http://localhost:8080/metrics',
  'http://localhost:8081/api/v1/monitoring/adapters/status',
  'http://192.168.2.71:8080/metrics',
  'http://192.168.2.71:8081/api/v1/monitoring/adapters/status'
];

async function testConnection(url) {
  try {
    const response = await fetch(url);
    console.log(`✅ ${url} - Status: ${response.status}`);
    if (url.includes('metrics')) {
      const data = await response.json();
      console.log(`   Gateway sinks: ${data.gateway?.total_sinks || 'N/A'}/${data.gateway?.running_sinks || 'N/A'}`);
    }
  } catch (error) {
    console.log(`❌ ${url} - Error: ${error.message}`);
  }
}

async function runTests() {
  console.log('Testing network connections...\n');
  
  for (const url of testUrls) {
    await testConnection(url);
  }
  
  console.log('\n测试完成！');
  console.log('如果localhost连接失败，请在.env文件中使用WSL IP地址。');
}

runTests();