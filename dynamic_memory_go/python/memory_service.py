import grpc
from concurrent import futures
import time
import numpy as np

# 导入pb文件，这里路径你可以自己改
import sys
sys.path.append('../proto') # 为了能导入pb，根据你的目录调
import memory_meta_pb2
import memory_meta_pb2_grpc

# 基础的元数据计算类，你可以继承这个改自己的逻辑
class BaseMetaCalculator:
    def calculate_weight(self, raw_input: str) -> float:
        # 算权重，你可以自己重写这个
        # 默认的简单实现：有核心标签的权重就高
        core_tags = ["core", "identity", "key_business"]
        for tag in core_tags:
            if tag in raw_input:
                return 0.8
        # 普通的就0.2
        return 0.2
    
    def extract_summary(self, raw_input: str) -> str:
        # 提摘要，你可以自己改，比如接个摘要模型
        # 默认的就截断长文本
        if len(raw_input) > 50:
            return raw_input[:50] + "..."
        return raw_input
    
    def extract_vector(self, raw_input: str) -> list[float]:
        # 提向量，你可以自己改，比如接你的向量模型
        # 默认的就整个随机的，演示用的
        return list(np.random.rand(384).astype(float))
    
    def get_tags(self, raw_input: str) -> list[str]:
        # 提标签，你可以自己改
        tags = []
        core_tags = ["core", "identity", "key_business"]
        for tag in core_tags:
            if tag in raw_input:
                tags.append(tag)
        if not tags:
            tags.append("daily")
        return tags
    
    def integrate_memories(self, memories: list) -> dict:
        # 整合记忆，你可以自己改这个逻辑
        # 默认的简单实现：拿最高权重的内容，合并标签，平均向量
        if not memories:
            return {}
        best_mem = max(memories, key=lambda x: x.weight)
        all_tags = list(set([tag for mem in memories for tag in mem.tags]))
        avg_vector = np.mean([mem.vector for mem in memories], axis=0).tolist()
        new_weight = best_mem.weight * 1.1
        
        return {
            "content": best_mem.content,
            "tags": all_tags,
            "vector": avg_vector,
            "weight": new_weight
        }

# 元数据服务的实现
class MemoryMetaServicer(memory_meta_pb2_grpc.MemoryMetaServiceServicer):
    def __init__(self):
        self.calculator = BaseMetaCalculator()
    
    def CalculateMemoryMeta(self, request, context):
        raw_input = request.raw_input
        
        # 算各项元数据
        weight = self.calculator.calculate_weight(raw_input)
        summary = self.calculator.extract_summary(raw_input)
        vector = self.calculator.extract_vector(raw_input)
        tags = self.calculator.get_tags(raw_input)
        
        return memory_meta_pb2.MemoryMetaResponse(
            weight=weight,
            semantic_summary=summary,
            vector=vector,
            tags=tags
        )
    
    def ExtractVector(self, request, context):
        input_text = request.input
        vector = self.calculator.extract_vector(input_text)
        return memory_meta_pb2.VectorResponse(vector=vector)
    
    def IntegrateMemories(self, request, context):
        mem_ids = request.memory_ids
        # 这里你自己写从存储拿记忆的逻辑
        similar_mems = [] # 示例，从数据库拿相似的
        
        integrated = self.calculator.integrate_memories(similar_mems)
        
        return memory_meta_pb2.IntegrateResponse(
            content=integrated.get("content", ""),
            tags=integrated.get("tags", []),
            vector=integrated.get("vector", []),
            weight=integrated.get("weight", 0.0)
        )

# 起服务
def serve():
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
    memory_meta_pb2_grpc.add_MemoryMetaServiceServicer_to_server(
        MemoryMetaServicer(), server
    )
    server.add_insecure_port('[::]:50051')
    print("元数据服务起好了，端口50051")
    server.start()
    try:
        while True:
            time.sleep(86400)
    except KeyboardInterrupt:
        server.stop(0)

if __name__ == '__main__':
    serve()
