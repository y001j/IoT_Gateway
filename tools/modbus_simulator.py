#!/usr/bin/env python3
"""
Modbus TCP服务器模拟器
用于测试IoT网关的Modbus适配器
"""

import logging
import random
import time
from pymodbus.server import StartTcpServer
from pymodbus.datastore import ModbusSequentialDataBlock
from pymodbus.datastore import ModbusSlaveContext, ModbusServerContext

# 配置日志
logging.basicConfig()
log = logging.getLogger()
log.setLevel(logging.INFO)

# 创建数据存储
def setup_data_store():
    # 初始化保持寄存器(Holding Registers)，起始地址为0，初始值为0
    hr = ModbusSequentialDataBlock(0, [0] * 100)
    
    # 初始化输入寄存器(Input Registers)，起始地址为0，初始值为0
    ir = ModbusSequentialDataBlock(0, [0] * 100)
    
    # 初始化线圈(Coils)，起始地址为0，初始值为0
    co = ModbusSequentialDataBlock(0, [0] * 100)
    
    # 初始化离散输入(Discrete Inputs)，起始地址为0，初始值为0
    di = ModbusSequentialDataBlock(0, [0] * 100)
    
    # 创建从站上下文
    store = ModbusSlaveContext(
        di=di,  # 离散输入
        co=co,  # 线圈
        hr=hr,  # 保持寄存器
        ir=ir   # 输入寄存器
    )
    
    # 创建服务器上下文，单个从站，ID为1
    context = ModbusServerContext(slaves=store, single=True)
    return context

# 更新模拟数据
def update_data(context):
    """
    定期更新寄存器中的数据，模拟真实设备
    """
    slave_id = 0x01  # 从站ID
    
    while True:
        # 更新温度值 (保持寄存器 0)
        temperature = random.uniform(20.0, 30.0)
        # 将浮点数转换为整数，乘以10保留一位小数
        temp_value = int(temperature * 10)
        context[slave_id].setValues(3, 0, [temp_value])
        
        # 更新湿度值 (保持寄存器 1)
        humidity = random.uniform(40.0, 80.0)
        hum_value = int(humidity * 10)
        context[slave_id].setValues(3, 1, [hum_value])
        
        # 更新压力值 (保持寄存器 2)
        pressure = random.uniform(1000.0, 1020.0)
        press_value = int(pressure)
        context[slave_id].setValues(3, 2, [press_value])
        
        # 更新状态值 (线圈 0)
        status = random.choice([0, 1])
        context[slave_id].setValues(1, 0, [status])
        
        # 更新报警值 (离散输入 0)
        alarm = 1 if temperature > 28.0 else 0
        context[slave_id].setValues(2, 0, [alarm])
        
        # 输出当前值
        log.info(f"更新模拟数据: 温度={temperature:.1f}°C, 湿度={humidity:.1f}%, "
                 f"压力={pressure:.1f}hPa, 状态={status}, 报警={alarm}")
        
        # 等待1秒
        time.sleep(1)

if __name__ == "__main__":
    # 创建数据存储
    context = setup_data_store()
    
    # 启动数据更新线程
    import threading
    t = threading.Thread(target=update_data, args=(context,))
    t.daemon = True
    t.start()
    
    # 启动Modbus TCP服务器
    log.info("启动Modbus TCP服务器，地址=0.0.0.0，端口=502")
    StartTcpServer(context, address=("0.0.0.0", 502))
