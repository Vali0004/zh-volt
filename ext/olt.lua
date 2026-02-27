-- Define o protocolo
local olt_proto = Proto("olt", "Custom OLT Protocol")

-- Define os campos do cabeçalho com base no main.go
local f_magic   = ProtoField.bytes("olt.magic", "Magic", base.SPACE)
local f_request = ProtoField.uint16("olt.request", "Request ID", base.HEX)
local f_type    = ProtoField.uint16("olt.type", "Type", base.HEX)
local f_status  = ProtoField.uint8("olt.status", "Status", base.HEX)
local f_reserv  = ProtoField.uint8("olt.reserv", "Reserved", base.HEX)
local f_check0  = ProtoField.uint8("olt.check0", "Check0", base.HEX)
local f_check1  = ProtoField.uint8("olt.check1", "Check1", base.HEX)
local f_data    = ProtoField.bytes("olt.data", "Data Payload", base.SPACE)

-- Registra os campos no protocolo
olt_proto.fields = { f_magic, f_request, f_type, f_status, f_reserv, f_check0, f_check1, f_data }

-- Função principal do Dissector
function olt_proto.dissector(buffer, pinfo, tree)
local length = buffer:len()

-- O cabeçalho mínimo tem 12 bytes (4 Magic + 2 Request + 2 Type + 4 Status/Checks)
if length < 12 then return end

  -- Verifica os "Magic bytes" (b9 58 d6 3a) para garantir que é o nosso pacote
  local magic_tvb = buffer(0, 4)
  if magic_tvb:uint() ~= 0xb958d63a then
    return -- Não é o protocolo OLT, ignora
    end

    -- Define o nome do protocolo na coluna principal do Wireshark
    pinfo.cols.protocol = "OLT"

    -- Cria a árvore principal no painel de detalhes do pacote
    local subtree = tree:add(olt_proto, buffer(), "OLT Protocol Data")

    -- Adiciona os campos à árvore com os offsets exatos do seu código Go
    subtree:add(f_magic, buffer(0, 4))
    subtree:add(f_request, buffer(4, 2))

    local type_val = buffer(6, 2):uint()
    local type_node = subtree:add(f_type, buffer(6, 2))

    -- Mapeia os Types conhecidos nos logs do código para facilitar a leitura
    if type_val == 0x0011 then type_node:append_text(" (Firmware/MAC Response)")
      elseif type_val == 0x0014 then type_node:append_text(" (ONU SN Info)")
        elseif type_val == 0x000d then type_node:append_text(" (ONU Keepalive/Ack)")
          elseif type_val == 0x0012 then type_node:append_text(" (OLT Uptime/Heartbeat)")
            elseif type_val == 0x010c then type_node:append_text(" (OLT Info)")
              end

              subtree:add(f_status, buffer(8, 1))
              subtree:add(f_reserv, buffer(9, 1))
              subtree:add(f_check0, buffer(10, 1))
              subtree:add(f_check1, buffer(11, 1))

              -- O que sobrar depois do byte 11 é payload (data)
              if length > 12 then
                subtree:add(f_data, buffer(12, length - 12))
                end
                end

                -- Associa este Dissector ao EtherType especificado no código (EthernetOltType = 0x88b6)
                local eth_table = DissectorTable.get("ethertype")
                eth_table:add(0x88b6, olt_proto)