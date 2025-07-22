// 测试插件端点的脚本
const testToken = 'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE3NTI2NzkwOTUsInJvbGUiOiJhZG1pbmlzdHJhdG9yIiwidXNlcl9pZCI6MSwidXNlcm5hbWUiOiJhZG1pbiJ9.joKnF6h4SIL76FBMgTYaxm9HO-dKh_I3FKZTUsYd0VM';

async function testPluginsEndpoint() {
  try {
    const response = await fetch('http://localhost:8081/api/v1/plugins', {
      headers: {
        'Authorization': `Bearer ${testToken}`,
        'Content-Type': 'application/json'
      }
    });
    
    if (!response.ok) {
      throw new Error(`HTTP ${response.status}: ${response.statusText}`);
    }
    
    const data = await response.json();
    console.log('✅ 插件端点测试成功！');
    console.log('总插件数:', data.data.data.length);
    
    const adapters = data.data.data.filter(p => p.type === 'adapter');
    const sinks = data.data.data.filter(p => p.type === 'sink');
    
    console.log('\n📊 插件统计:');
    console.log('适配器数量:', adapters.length);
    console.log('连接器数量:', sinks.length);
    
    console.log('\n🔌 适配器列表:');
    adapters.forEach(adapter => {
      console.log(`  - ${adapter.name}: ${adapter.status} (${adapter.description})`);
    });
    
    console.log('\n🔗 连接器列表:');
    sinks.forEach(sink => {
      console.log(`  - ${sink.name}: ${sink.status} (${sink.description})`);
    });
    
    console.log('\n▶️ 运行中的插件:');
    const running = data.data.data.filter(p => p.status === 'running');
    running.forEach(plugin => {
      console.log(`  - ${plugin.name} (${plugin.type}): ${plugin.status}`);
    });
    
  } catch (error) {
    console.error('❌ 测试失败:', error.message);
  }
}

testPluginsEndpoint();